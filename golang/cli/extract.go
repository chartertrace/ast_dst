// Package cli implements the astdst command, shared by the `go run ./cmd`
// entrypoint and the installable `cmd/astdst` binary so both expose the same two
// modes:
//
//	astdst [flags]              extract the structural model (default)
//	astdst generate [flags]     generate a TLC-verified TLA+ spec, then extract
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

// defaultRoot is used when neither --root nor a config supplies one.
const defaultRoot = "../../primary-server/sim"

// Main is the command entrypoint: it dispatches the "generate" subcommand and
// otherwise runs the default structural extraction.
func Main(args []string) {
	if len(args) > 0 && args[0] == "generate" {
		RunGenerate(args[1:])
		return
	}
	RunExtract(args)
}

// RunExtract parses a codebase and emits the structural model.
func RunExtract(args []string) {
	fs := flag.NewFlagSet("astdst", flag.ExitOnError)
	root := fs.String("root", "", "codebase root to parse (overrides config; default sim path)")
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
	fmt.Fprintf(os.Stderr,
		"astdst: %d truths, %d invariants, %d faults, %d operations, %d edges\n",
		model.Stats.Truths, model.Stats.Invariants, model.Stats.Faults,
		model.Stats.Operations, model.Stats.Edges)
}

// loadConfig resolves the config the same way for both subcommands.
func loadConfig(configPath, root string) extract.Config {
	cfg := extract.DefaultConfig()
	if configPath != "" {
		loaded, err := extract.LoadConfig(configPath)
		if err != nil {
			fail("config: %v", err)
		}
		cfg = loaded
	}
	if root != "" {
		cfg.Root = root
	}
	if cfg.Root == "" {
		cfg.Root = defaultRoot
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
