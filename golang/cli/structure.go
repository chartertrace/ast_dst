package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/chartertrace/ast_dst/golang/extract"
)

// RunStructure extracts a generic Go-structure model of any codebase (functions,
// types, and package-level globals, plus the references between them) and emits
// it in the same model.json the viewer renders. Unlike the default mode it needs
// no sim config — point --root at any module, including ast_dst itself.
func RunStructure(args []string) {
	fs := flag.NewFlagSet("astdst structure", flag.ExitOnError)
	root := fs.String("root", "", "Go module/dir to analyse (required; recurses all packages)")
	out := fs.String("out", "", "output file (default: stdout)")
	indent := fs.Bool("indent", true, "pretty-print JSON")
	exportedOnly := fs.Bool("exported-only", false, "include only exported declarations")
	_ = fs.Parse(args)

	if *root == "" {
		fail("structure: --root is required (the Go module/dir to analyse)")
	}

	model, err := extract.ExtractStructure(extract.StructureConfig{
		Root:         *root,
		ExportedOnly: *exportedOnly,
	})
	if err != nil {
		fail("%v", err)
	}
	if err := write(*out, *indent, model); err != nil {
		fail("write: %v", err)
	}
	fmt.Fprintf(os.Stderr,
		"astdst structure: %d packages, %d functions, %d types, %d vars/consts, %d edges\n",
		model.Stats.Truths, model.Stats.Operations, model.Stats.Faults,
		model.Stats.Invariants, model.Stats.Edges)
}
