package extract

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
	"time"
)

// StructureConfig drives the generic, non-DST extraction mode: it works on any
// Go module, not just a sim-shaped one. Root is the module/dir to parse;
// Packages restricts to sub-directories (empty = recurse the whole tree);
// ExportedOnly drops unexported declarations.
type StructureConfig struct {
	Root         string
	Packages     []string
	ExportedOnly bool
}

// ExtractStructure parses an arbitrary Go codebase and returns the same Model
// the viewer renders, but populated with a language-structure mapping rather
// than DST concepts:
//
//   - operations  ← top-level functions & methods   (col 1, "Functions")
//   - faults      ← declared types                   (col 2, "Types")
//   - truths→inv  ← packages → their vars & consts    (col 3, "Packages")
//   - op→fault    ← a function references a type in its signature
//
// These three declaration kinds are disjoint, so nothing is double-counted. Type
// references are matched by name (go/ast carries no type resolution): a function
// is linked to a type when its signature names that type. A missed link is a
// missing edge, never an invented one — the same honesty rule the DST path uses.
func ExtractStructure(cfg StructureConfig) (*Model, error) {
	if cfg.Root == "" {
		return nil, fmt.Errorf("structure: root is empty")
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

	keep := func(name string) bool { return !cfg.ExportedOnly || ast.IsExported(name) }

	// First pass: the set of declared type names, so signatures can be matched.
	typeID := map[string]string{} // base type name -> "pkg.Type" id
	forEachFile(p, func(pkgName string, f *ast.File) {
		forEachGenSpec(f, token.TYPE, func(spec ast.Spec) {
			ts, ok := spec.(*ast.TypeSpec)
			if ok && keep(ts.Name.Name) {
				typeID[ts.Name.Name] = pkgName + "." + ts.Name.Name
			}
		})
	})

	faults := structureTypes(p, keep)
	operations := structureFuncs(p, keep, typeID)
	invariants, truths := structurePackageGlobals(p, keep)

	m := &Model{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Source:      Source{SimPath: cfg.Root, FilesRead: len(p.files), ToolModule: "astdst"},
		Truths:      truths,
		Invariants:  invariants,
		Faults:      faults,
		Operations:  operations,
		Meta: &Meta{
			Mode:  "structure",
			Title: "Go structure",
			Labels: Labels{
				Operations: "Functions",
				Faults:     "Types",
				Invariants: "Vars & consts",
				Truths:     "Packages",
			},
		},
	}
	m.Edges = buildEdges(m)
	m.Stats = Stats{
		Truths:      len(truths),
		Invariants:  len(invariants),
		Faults:      len(faults),
		Operations:  len(operations),
		TotalWeight: totalWeight(operations),
		Edges:       len(m.Edges),
	}
	return m, nil
}

// structureTypes turns every declared type into a "fault" node: enum is the bare
// type name (so the viewer's existing chip + injector-count plumbing keys off
// it), id is package-qualified to stay unique across packages.
func structureTypes(p *pkg, keep func(string) bool) []Fault {
	var out []Fault
	forEachFile(p, func(pkgName string, f *ast.File) {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || !keep(ts.Name.Name) {
					continue
				}
				doc := docText(ts.Doc)
				if doc == "" {
					doc = docText(gd.Doc)
				}
				out = append(out, Fault{
					ID:    pkgName + "." + ts.Name.Name,
					Enum:  ts.Name.Name,
					Label: ts.Name.Name + "  (" + pkgName + " " + typeKind(ts.Type) + ")",
					Doc:   doc,
					Loc:   p.loc(ts.Pos()),
				})
			}
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// structureFuncs turns every top-level function/method into an "operation":
// weight is its body length in lines, note its signature, and faults the set of
// declared types its signature names.
func structureFuncs(p *pkg, keep func(string) bool, typeID map[string]string) []Operation {
	var out []Operation
	forEachFile(p, func(pkgName string, f *ast.File) {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !keep(fn.Name.Name) {
				continue
			}
			name := fn.Name.Name
			if recv := receiverType(fn); recv != "" {
				name = recv + "." + name
			}
			refs := referencedTypes(fn, typeID)
			if refs == nil {
				refs = []string{} // marshal as [] not null; the viewer maps over it
			}
			out = append(out, Operation{
				Handler: pkgName + "." + name,
				Name:    name,
				Weight:  float64(bodyLines(p, fn)),
				Note:    signature(p, fn),
				Faults:  refs,
				Loc:     p.loc(fn.Pos()),
			})
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Handler < out[j].Handler })
	for i := range out {
		out[i].Index = i
	}
	// Share is weight / total, matching the DST bar semantics.
	var total float64
	for _, o := range out {
		total += o.Weight
	}
	if total > 0 {
		for i := range out {
			out[i].Share = out[i].Weight / total
		}
	}
	return out
}

// structurePackageGlobals turns each package-level var/const into an "invariant"
// grouped under its package ("truth"). Only packages that declare at least one
// global appear as a truth — the same rule the DST path uses (no empty groups).
func structurePackageGlobals(p *pkg, keep func(string) bool) ([]Invariant, []Truth) {
	var out []Invariant
	count := map[string]int{}
	var order []string
	forEachFile(p, func(pkgName string, f *ast.File) {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || (gd.Tok != token.VAR && gd.Tok != token.CONST) {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, n := range vs.Names {
					if n.Name == "_" || !keep(n.Name) {
						continue
					}
					doc := docText(vs.Doc)
					if doc == "" {
						doc = docText(gd.Doc)
					}
					if count[pkgName] == 0 {
						order = append(order, pkgName)
					}
					count[pkgName]++
					out = append(out, Invariant{
						ID:    pkgName + "." + n.Name,
						Label: n.Name,
						Truth: pkgName,
						Doc:   doc,
						Loc:   p.loc(n.Pos()),
					})
				}
			}
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	sort.Strings(order)
	truths := make([]Truth, 0, len(order))
	for _, name := range order {
		truths = append(truths, Truth{Name: name, Count: count[name]})
	}
	return out, truths
}

// --- helpers ----------------------------------------------------------------

// forEachFile visits parsed files in deterministic (path-sorted) order, passing
// each file's package name.
func forEachFile(p *pkg, fn func(pkgName string, f *ast.File)) {
	paths := make([]string, 0, len(p.files))
	for path := range p.files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		f := p.files[path]
		name := ""
		if f.Name != nil {
			name = f.Name.Name
		}
		fn(name, f)
	}
}

// forEachGenSpec visits every spec of a top-level GenDecl of the given token.
func forEachGenSpec(f *ast.File, tok token.Token, fn func(ast.Spec)) {
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != tok {
			continue
		}
		for _, spec := range gd.Specs {
			fn(spec)
		}
	}
}

// receiverType returns the base type name of a method's receiver ("Engine" for
// `func (e *Engine) M()`), or "" for a free function.
func receiverType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	return identName(deref(fn.Recv.List[0].Type))
}

func deref(e ast.Expr) ast.Expr {
	if star, ok := e.(*ast.StarExpr); ok {
		return star.X
	}
	return e
}

// referencedTypes returns the declared-type names a function's signature names
// (receiver, params, results), as bare names that match the viewer's chips.
func referencedTypes(fn *ast.FuncDecl, typeID map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	visit := func(e ast.Expr) {
		if e == nil {
			return
		}
		ast.Inspect(e, func(n ast.Node) bool {
			var name string
			switch v := n.(type) {
			case *ast.SelectorExpr:
				name = v.Sel.Name
			case *ast.Ident:
				name = v.Name
			}
			if name != "" && typeID[name] != "" && !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
			return true
		})
	}
	if fn.Recv != nil {
		for _, f := range fn.Recv.List {
			visit(f.Type)
		}
	}
	if fn.Type != nil {
		if fn.Type.Params != nil {
			for _, f := range fn.Type.Params.List {
				visit(f.Type)
			}
		}
		if fn.Type.Results != nil {
			for _, f := range fn.Type.Results.List {
				visit(f.Type)
			}
		}
	}
	sort.Strings(out)
	return out
}

// signature renders a function's parameter/result signature as Go text.
func signature(p *pkg, fn *ast.FuncDecl) string {
	sig := "(" + p.renderFieldList(fn.Type.Params) + ")"
	if res := p.renderFieldList(fn.Type.Results); res != "" {
		if fn.Type.Results != nil && len(fn.Type.Results.List) > 1 {
			sig += " (" + res + ")"
		} else {
			sig += " " + res
		}
	}
	return sig
}

// bodyLines is a function's body length in source lines (≥1), used as its weight.
func bodyLines(p *pkg, fn *ast.FuncDecl) int {
	if fn.Body == nil {
		return 1
	}
	n := p.line(fn.Body.End()) - p.line(fn.Body.Pos()) + 1
	if n < 1 {
		return 1
	}
	return n
}

// typeKind labels a type spec for display: struct, interface, alias, or type.
func typeKind(e ast.Expr) string {
	switch e.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	case *ast.Ident, *ast.SelectorExpr:
		return "alias"
	default:
		return "type"
	}
}
