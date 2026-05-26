// Package cli implements the astdst command, shared by the `go run ./cmd`
// entrypoint and the installable `cmd/astdst` binary so both expose the same two
// modes:
//
//	astdst [flags]                    extract the DST model (default)
//	astdst generate [flags]           generate a TLA+ spec (TLC-verified when the toolchain is present), then extract
//	astdst structure [flags]          extract a generic Go-structure model of any repo
//	astdst doctor [flags]             report whether the TLA+ verification toolchain is wired up
//	astdst trace-compact [flags]      compact a DST NDJSON trace into Parquet for the flow viewer
//
// With no --config it uses the built-in preset for CharterTrace's
// primary-server/sim. Point it at any sim-shaped codebase by passing a config
// that names that repo's catalogue vars, enum type, checker pattern, and trace
// call — see ../presets/sim.json for the schema.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/chartertrace/ast_dst/golang/extract"
)

// Main is the command entrypoint: it dispatches the "generate" subcommand and
// otherwise runs the default structural extraction.
func Main(args []string) {
	if len(args) > 0 && args[0] == "generate" {
		RunGenerate(args[1:])
		return
	}
	if len(args) > 0 && args[0] == "structure" {
		RunStructure(args[1:])
		return
	}
	if len(args) > 0 && args[0] == "doctor" {
		RunDoctor(args[1:])
		return
	}
	if len(args) > 0 && args[0] == "trace-compact" {
		RunTraceCompact(args[1:])
		return
	}
	RunExtract(args)
}

// RunExtract parses a codebase and emits the structural model.
func RunExtract(args []string) {
	fs := flag.NewFlagSet("astdst", flag.ExitOnError)
	root := fs.String("root", "", "codebase root to parse (overrides config; default: current directory)")
	configPath := fs.String("config", "", "JSON config file (default: built-in sim preset)")
	out := fs.String("out", "", "output file (default: stdout)")
	indent := fs.Bool("indent", true, "pretty-print JSON")
	_ = fs.Parse(args)

	cfg := loadConfig(*configPath, *root)
	model, err := extract.Extract(cfg)
	if err != nil {
		fail("%v", err)
	}
	if err := write(*out, *indent, model); err != nil {
		fail("write: %v", err)
	}
	warnUnresolved(cfg, model)
	fmt.Fprintf(os.Stderr,
		"astdst: %d truths, %d invariants, %d faults, %d operations, %d edges\n",
		model.Stats.Truths, model.Stats.Invariants, model.Stats.Faults,
		model.Stats.Operations, model.Stats.Edges)
}

// warnUnresolved flags a configured catalogue name that matched nothing. Without
// it, "this repo has no operations" and "the configured tableVar is wrong" both
// render as an empty section — indistinguishable, which undercuts the tool's
// honesty guarantee (a gap should be a real gap, not a silent misconfig). The
// check is a heuristic (a configured name that produced zero results), stderr-
// only and non-fatal, so it never blocks a legitimately empty extraction.
func warnUnresolved(cfg extract.Config, m *extract.Model) {
	warn := func(concept, name string) {
		fmt.Fprintf(os.Stderr,
			"astdst: warning: configured %s %q matched nothing — check the name or --root\n",
			concept, name)
	}
	if cfg.Faults.StructSliceVar != "" || cfg.Faults.EnumType != "" {
		if len(m.Faults) == 0 {
			name := cfg.Faults.StructSliceVar
			if name == "" {
				name = cfg.Faults.EnumType
			}
			warn("fault source", name)
		}
	}
	if cfg.Invariants.StructSliceVar != "" && len(m.Invariants) == 0 {
		warn("invariants.structSliceVar", cfg.Invariants.StructSliceVar)
	}
	if cfg.Operations.TableVar != "" && len(m.Operations) == 0 {
		warn("operations.tableVar", cfg.Operations.TableVar)
	}
	if cfg.State.TypeName != "" && m.State == nil {
		warn("state.typeName", cfg.State.TypeName)
	}
	if cfg.TLA.SpecDir != "" && len(m.Specs) == 0 {
		warn("tla.specDir", cfg.TLA.SpecDir)
	}
}

// loadConfig resolves the config the same way for both subcommands. With neither
// --root nor a config-supplied root, it defaults to the current directory, so the
// tool runs on whatever repo you invoke it in; if that holds no Go files the
// extractor errors loudly ("no Go files found under …") rather than silently
// analysing a path the user never chose.
//
// When no --config is given we fall back to the built-in sim preset's *names*
// but drop its package allowlist (catalog/engine/state): those dirs only exist
// in CharterTrace's sim, and demanding them would make a bare run on any other
// repo fail outright instead of degrading. With no allowlist the extractor scans
// the whole tree, finds no sim-shaped declarations, and reports an honest empty
// model (warnUnresolved explains which configured names matched nothing).
func loadConfig(configPath, root string) extract.Config {
	cfg := extract.DefaultConfig()
	if configPath != "" {
		loaded, err := extract.LoadConfig(configPath)
		if err != nil {
			fail("config: %v", err)
		}
		cfg = loaded
	} else {
		cfg.Packages = nil
	}
	if root != "" {
		cfg.Root = root
	}
	if cfg.Root == "" {
		cfg.Root = "."
	}
	return cfg
}

func write(path string, indent bool, model *extract.Model) error {
	var data []byte
	var err error
	if indent {
		data, err = json.MarshalIndent(model, "", "  ")
	} else {
		data, err = json.Marshal(model)
	}
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if path == "" {
		_, err = os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "astdst: "+format+"\n", args...)
	os.Exit(1)
}
