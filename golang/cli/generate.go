package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chartertrace/ast_dst/golang/extract"
	"github.com/chartertrace/ast_dst/golang/gen"
)

// RunGenerate deterministically synthesises a TLA+ spec from the codebase (no
// LLM, no network), verifies it with the real tools (SANY then TLC), writes it,
// and re-extracts the model with that spec bound in — so the generated spec flows
// through the exact same pipeline (and viewer) as a hand-written one. Requires
// only java and tla2tools.jar.
func RunGenerate(args []string) {
	fs := flag.NewFlagSet("astdst generate", flag.ExitOnError)
	root := fs.String("root", "", "codebase root to parse (overrides config; default sim path)")
	configPath := fs.String("config", "", "JSON config file (default: built-in sim preset)")
	outModel := fs.String("out", "", "model.json output file (default: stdout)")
	outSpec := fs.String("out-spec", "./generated", "directory to write the generated .tla/.cfg")
	module := fs.String("module", "DstSpec", "TLA+ module name (also the file base name)")
	jar := fs.String("jar", "", "path to tla2tools.jar (else $TLA2TOOLS_JAR or beside the spec)")
	workers := fs.Int("workers", 2, "TLC parallel workers")
	timeout := fs.Duration("timeout", 2*time.Minute, "wall-clock budget for verification")
	indent := fs.Bool("indent", true, "pretty-print JSON")
	_ = fs.Parse(args)

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
	searchDir := cfg.TLA.SpecDir
	if searchDir != "" && !filepath.IsAbs(searchDir) {
		searchDir = filepath.Join(absRoot, searchDir)
	}
	tc, err := gen.NewToolchain(*jar, searchDir)
	if err != nil {
		fail("toolchain: %v", err)
	}

	// 3. Synthesise + verify deterministically.
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	fmt.Fprintln(os.Stderr, "astdst: synthesising TLA+ spec and verifying with SANY + TLC…")
	res, err := gen.Generate(ctx, tc, model, gen.Options{
		ModuleName: *module,
		Workers:    *workers,
	})
	if err != nil {
		fail("generate: %v", err)
	}

	// 4. Persist the spec (verified or not — reported honestly either way).
	if err := writeSpec(*outSpec, res.Draft); err != nil {
		fail("write spec: %v", err)
	}
	reportVerification(res, *outSpec)

	// 5. Re-extract with the generated spec bound in, stamp provenance, emit.
	cfg.TLA.SpecDir = *outSpec
	cfg.TLA.CfgFile = res.Draft.Module + ".cfg"
	cfg.TLA.MatchByLabel = true
	out, err := extract.Extract(cfg)
	if err != nil {
		fail("re-extract with generated spec: %v", err)
	}
	states, depth := 0, 0
	if res.TLC != nil {
		states, depth = res.TLC.States, res.TLC.Depth
	}
	extract.MarkGenerated(out, res.Verified, states, depth)

	if err := write(*outModel, *indent, out); err != nil {
		fail("write model: %v", err)
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
func reportVerification(res *gen.GenResult, dir string) {
	r := res.Report
	fmt.Fprintf(os.Stderr,
		"astdst: synthesised %d vars (%d typed), %d ops (%d transitions / %d stub), invariants: %d active / %d reference / %d stub\n",
		r.Variables, r.Typed, r.Operations, r.Transitions, r.StubOps, r.ActiveInvs, r.ReferenceInvs, r.StubInvs)
	switch {
	case res.Verified:
		fmt.Fprintf(os.Stderr,
			"astdst: ✓ VERIFIED by TLC (well-formed; initial state satisfies TypeOK + %d active invariant(s)) → %s\n",
			res.Report.ActiveInvs+1, dir)
	case res.SANY != nil && !res.SANY.OK:
		fmt.Fprintf(os.Stderr, "astdst: ✗ UNVERIFIED — spec failed to parse (SANY) → %s\n", dir)
	case res.TLC != nil && res.TLC.Violated != "":
		fmt.Fprintf(os.Stderr, "astdst: ✗ UNVERIFIED — invariant %s does not hold at the initial state → %s\n", res.TLC.Violated, dir)
	case res.TLC != nil && res.TLC.TimedOut:
		fmt.Fprintf(os.Stderr, "astdst: ✗ UNVERIFIED — TLC timed out → %s\n", dir)
	default:
		fmt.Fprintf(os.Stderr, "astdst: ✗ UNVERIFIED → %s\n", dir)
	}
}
