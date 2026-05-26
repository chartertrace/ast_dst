package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chartertrace/ast_dst/golang/extract"
	"github.com/chartertrace/ast_dst/golang/gen"
)

// RunGenerate deterministically synthesises a TLA+ spec from the codebase (no
// LLM, no network), writes it, and re-extracts the model with that spec bound in
// — so the generated spec flows through the exact same pipeline (and viewer) as a
// hand-written one. Synthesis itself has no dependencies. Verification (SANY then
// TLC) is an optional enhancement: with --verify (the default) it runs when a JVM
// + tla2tools.jar are present, and otherwise the spec is still written, clearly
// marked unverified. Pass --verify=false to skip the toolchain lookup entirely.
func RunGenerate(args []string) {
	fs := flag.NewFlagSet("astdst generate", flag.ExitOnError)
	root := fs.String("root", "", "codebase root to parse (overrides config; default sim path)")
	configPath := fs.String("config", "", "JSON config file (default: built-in sim preset)")
	outModel := fs.String("out", "", "model.json output file (default: stdout)")
	outSpec := fs.String("out-spec", "./generated", "directory to write the generated .tla/.cfg")
	module := fs.String("module", "DstSpec", "TLA+ module name (also the file base name)")
	jar := fs.String("jar", "", "path to tla2tools.jar (else $TLA2TOOLS_JAR or beside the spec)")
	verify := fs.Bool("verify", true, "verify with SANY+TLC when a JVM + tla2tools.jar are available")
	requireVerified := fs.Bool("require-verified", false, "exit non-zero unless TLC verified the spec clean (CI gating; implies --verify)")
	maxNat := fs.Int("max-nat", 2, "bound for numeric state vars (0..N) during TLC; larger = deeper but slower")
	workers := fs.Int("workers", 2, "TLC parallel workers")
	timeout := fs.Duration("timeout", 2*time.Minute, "wall-clock budget for verification")
	indent := fs.Bool("indent", true, "pretty-print JSON")
	_ = fs.Parse(args)

	if *requireVerified && !*verify {
		fail("--require-verified cannot be combined with --verify=false")
	}

	cfg := loadConfig(*configPath, *root)
	absRoot, err := filepath.Abs(cfg.Root)
	if err != nil {
		fail("resolve root: %v", err)
	}

	// 1. Extract the structural model (carries state, invariants, and any specs).
	model, err := extract.Extract(cfg)
	if err != nil {
		fail("extract: %v", err)
	}

	// 2. Locate the TLA+ toolchain (search the configured spec dir for the jar).
	// A missing toolchain is NOT fatal: synthesis is deterministic and needs no
	// JVM, so we still produce the spec — just unverified. Verification is an
	// optional enhancement, never a prerequisite for getting the spec.
	var tc *gen.Toolchain
	if *verify {
		searchDir := cfg.TLA.SpecDir
		if searchDir != "" && !filepath.IsAbs(searchDir) {
			searchDir = filepath.Join(absRoot, searchDir)
		}
		var tcErr error
		tc, tcErr = gen.NewToolchain(*jar, searchDir)
		if tcErr != nil {
			fmt.Fprintf(os.Stderr,
				"astdst: TLA+ toolchain unavailable (%v)\n"+
					"astdst: synthesising the spec anyway — it will be marked UNVERIFIED.\n"+
					"astdst: run `astdst doctor` to diagnose, then re-run to verify with TLC.\n",
				tcErr)
		}
	} else {
		fmt.Fprintln(os.Stderr, "astdst: --verify=false — skipping the toolchain; spec will be UNVERIFIED.")
	}

	// 3. Synthesise (always) + verify (only if the toolchain was found).
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if tc != nil {
		fmt.Fprintln(os.Stderr, "astdst: synthesising TLA+ spec and verifying with SANY + TLC…")
	} else {
		fmt.Fprintln(os.Stderr, "astdst: synthesising TLA+ spec (verification skipped)…")
	}
	res, err := gen.Generate(ctx, tc, model, gen.Options{
		ModuleName: *module,
		MaxNat:     *maxNat,
		Workers:    *workers,
	})
	if err != nil {
		fail("generate: %v", err)
	}

	// 4. Persist the spec (verified or not — reported honestly either way).
	if err := writeSpec(*outSpec, res.Draft); err != nil {
		fail("write spec: %v", err)
	}
	reportVerification(res, *outSpec, *maxNat)

	// 5. Re-extract with the generated spec bound in, stamp provenance, emit.
	cfg.TLA.SpecDir = *outSpec
	cfg.TLA.CfgFile = res.Draft.Module + ".cfg"
	cfg.TLA.MatchByLabel = true
	out, err := extract.Extract(cfg)
	if err != nil {
		fail("re-extract with generated spec: %v", err)
	}
	states, depth := 0, 0
	violated := ""
	var trace []extract.TraceState
	if res.TLC != nil {
		states, depth = res.TLC.States, res.TLC.Depth
		violated = res.TLC.Violated
		trace = toExtractTrace(res.TLC.Trace)
	}
	// Behavioral = TLC checked a real value transition against an active invariant,
	// so the ✓ means more than "well-formed".
	behavioral := res.Verified && res.Report.ActiveInvs > 0 && res.Report.ValueTransitions > 0
	extract.MarkGenerated(out, res.Verified, behavioral, states, depth, *maxNat, violated, trace)

	if err := write(*outModel, *indent, out); err != nil {
		fail("write model: %v", err)
	}

	// 6. CI gate: when the caller demands verification, a non-verified result
	// (toolchain absent, SANY/TLC failure, or a violation) is an error. The spec
	// and model are still written above for inspection; only the exit code
	// reflects that verification did not pass.
	if *requireVerified && !res.Verified {
		fail("--require-verified: spec is not TLC-verified (see the verdict above)")
	}
}

// writeSpec writes the draft's module and config into dir.
func writeSpec(dir string, d *gen.Draft) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, d.Module+".tla"), []byte(d.TLA), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, d.Module+".cfg"), []byte(d.CFG), 0o644)
}

// reportVerification prints an honest one-line verdict to stderr, including what
// the synthesis could and could not recover.
func reportVerification(res *gen.GenResult, dir string, maxNat int) {
	r := res.Report
	fmt.Fprintf(os.Stderr,
		"astdst: synthesised %d vars (%d typed), %d ops (%d transitions, %d with value forms / %d stub), invariants: %d active / %d reference / %d stub\n",
		r.Variables, r.Typed, r.Operations, r.Transitions, r.ValueTransitions, r.StubOps, r.ActiveInvs, r.ReferenceInvs, r.StubInvs)
	behavioral := res.Verified && r.ActiveInvs > 0 && r.ValueTransitions > 0
	switch {
	case res.Verified && behavioral:
		fmt.Fprintf(os.Stderr,
			"astdst: ✓ VERIFIED by TLC (behavioral: %d active invariant(s) checked against value transitions over %d states, bounded 0..%d) → %s\n",
			r.ActiveInvs+1, res.TLC.States, maxNat, dir)
	case res.Verified:
		fmt.Fprintf(os.Stderr,
			"astdst: ✓ verified WELL-FORMED by TLC (parses + holds over the modelled states, bounded 0..%d, but transitions carry no value semantics — a weak claim) → %s\n",
			maxNat, dir)
	case res.SANY != nil && !res.SANY.OK:
		fmt.Fprintf(os.Stderr, "astdst: ✗ UNVERIFIED — spec failed to parse (SANY) → %s\n", dir)
	case res.TLC != nil && res.TLC.Violated != "":
		fmt.Fprintf(os.Stderr, "astdst: ✗ UNVERIFIED — invariant %s violated (%d-step trace) → %s\n",
			res.TLC.Violated, len(res.TLC.Trace), dir)
		reportTrace(res.TLC.Trace)
	case res.TLC != nil && res.TLC.TimedOut:
		fmt.Fprintf(os.Stderr, "astdst: ✗ UNVERIFIED — TLC timed out → %s\n", dir)
	case res.SANY == nil && res.TLC == nil:
		fmt.Fprintf(os.Stderr,
			"astdst: ⚙✗ UNVERIFIED — verification did not run (no TLA+ toolchain). The spec is well-formed by construction but unchecked → %s\n",
			dir)
	default:
		fmt.Fprintf(os.Stderr, "astdst: ✗ UNVERIFIED → %s\n", dir)
	}
}

// reportTrace prints the counterexample path's final state (the most actionable
// part) to stderr; the full path is preserved in model.json's counterexample.
func reportTrace(trace []gen.TraceState) {
	if len(trace) == 0 {
		return
	}
	last := trace[len(trace)-1]
	keys := make([]string, 0, len(last.Vars))
	for k := range last.Vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s = %s", k, last.Vars[k])
	}
	fmt.Fprintf(os.Stderr, "astdst:   failing state (step %d): %s\n", last.Num, b.String())
}

// toExtractTrace converts gen.TraceState to the extract package's mirror type so
// it can be stamped into model.json without an extract→gen import cycle.
func toExtractTrace(trace []gen.TraceState) []extract.TraceState {
	if len(trace) == 0 {
		return nil
	}
	out := make([]extract.TraceState, len(trace))
	for i, s := range trace {
		out[i] = extract.TraceState{Num: s.Num, Action: s.Action, Vars: s.Vars}
	}
	return out
}
