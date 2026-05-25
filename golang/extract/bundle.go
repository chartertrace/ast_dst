package extract

import (
	"fmt"
	"go/token"
	"path/filepath"
	"strings"
)

// Bundle is the distilled, source-grounded view of a codebase handed to the TLA+
// generator: the mutable state, the operations as candidate transitions (each
// with its handler source so the model can read the actual mutations), the
// invariants to formalise, and the faults. It is deliberately compact — the
// structured model plus only the source snippets that matter — so it grounds the
// generator without shipping the whole repo.
type Bundle struct {
	Source     Source      `json:"source"`
	State      *StateModel `json:"state,omitempty"`
	Operations []OpSpec    `json:"operations"`
	Invariants []Invariant `json:"invariants"`
	Faults     []Fault     `json:"faults"`
}

// OpSpec is one operation enriched with the rendered Go source of its handler,
// so the generator can see which state the transition reads and writes rather
// than guessing from the name alone.
type OpSpec struct {
	Operation
	Source string `json:"source,omitempty"` // rendered handler body
}

// BuildBundle parses the codebase once and assembles the generator bundle. It
// reuses the same extractors as Extract, then attaches each operation's handler
// source. A configured-but-absent state type is left nil (a gap, not an error),
// matching the rest of the package's honesty about missing structure.
func BuildBundle(cfg Config) (*Bundle, error) {
	if cfg.Root == "" {
		return nil, fmt.Errorf("config root is empty")
	}
	absRoot, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	p, err := parseRoots(fset, absRoot, cfg.Packages)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", cfg.Root, err)
	}

	invariants, _ := extractInvariants(p, cfg.Invariants)
	operations := extractOperations(p, cfg.Operations)

	prefix := cfg.Operations.handlerPrefix()
	decls := p.findFuncs(func(n string) bool { return strings.HasPrefix(n, prefix) })
	ops := make([]OpSpec, 0, len(operations))
	for _, op := range operations {
		spec := OpSpec{Operation: op}
		if fn, ok := decls[op.Handler]; ok && fn != nil {
			spec.Source = p.renderNode(fn)
		}
		ops = append(ops, spec)
	}

	return &Bundle{
		Source: Source{
			SimPath:    cfg.Root,
			FilesRead:  len(p.files),
			ToolModule: "astdst",
		},
		State:      extractState(p, cfg.State),
		Operations: ops,
		Invariants: invariants,
		Faults:     extractFaults(p, cfg.Faults),
	}, nil
}
