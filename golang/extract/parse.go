package extract

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

// pkg is one parsed Go package: every non-test .go file in a directory, parsed
// with comments so doc strings survive into the model.
type pkg struct {
	fset    *token.FileSet
	files   map[string]*ast.File // path -> AST
	simRoot string               // for relative Loc paths
}

// parseDir parses every non-test .go file directly inside dir.
func parseDir(fset *token.FileSet, dir, simRoot string) (*pkg, error) {
	pkgs, err := parser.ParseDir(fset, dir, func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	p := &pkg{fset: fset, files: map[string]*ast.File{}, simRoot: simRoot}
	for _, ap := range pkgs {
		for path, f := range ap.Files {
			p.files[path] = f
		}
	}
	return p, nil
}

// parseRoots parses the requested packages under root into one merged pkg. If
// packages is empty it recurses into every directory under root that holds Go
// files, so the tool works on a codebase whose layout it knows nothing about.
// Locations are reported relative to root.
func parseRoots(fset *token.FileSet, root string, packages []string) (*pkg, error) {
	dirs := packages
	if len(dirs) == 0 {
		found, err := goDirs(root)
		if err != nil {
			return nil, err
		}
		dirs = found
	}
	merged := &pkg{fset: fset, files: map[string]*ast.File{}, simRoot: root}
	for _, d := range dirs {
		abs := d
		if !filepath.IsAbs(d) {
			abs = filepath.Join(root, d)
		}
		p, err := parseDir(fset, abs, root)
		if err != nil {
			return nil, err
		}
		for path, f := range p.files {
			merged.files[path] = f
		}
	}
	if len(merged.files) == 0 {
		return nil, fmt.Errorf("no Go files found under %s", root)
	}
	return merged, nil
}

// goDirs returns every directory at or below root that contains a non-test .go
// file, relative paths excluded so parseDir gets absolute dirs.
func goDirs(root string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") {
			dir := filepath.Dir(path)
			if !seen[dir] {
				seen[dir] = true
				out = append(out, dir)
			}
		}
		return nil
	})
	return out, err
}

// loc converts a token.Pos to a Loc with a sim-root-relative path.
func (p *pkg) loc(pos token.Pos) *Loc {
	if !pos.IsValid() {
		return nil
	}
	pp := p.fset.Position(pos)
	rel, err := filepath.Rel(p.simRoot, pp.Filename)
	if err != nil {
		rel = pp.Filename
	}
	return &Loc{File: rel, Line: pp.Line}
}

// line returns the 1-based line number for a position.
func (p *pkg) line(pos token.Pos) int {
	if !pos.IsValid() {
		return 0
	}
	return p.fset.Position(pos).Line
}

// findFuncs returns every top-level func/method declaration in the package
// whose name satisfies keep, keyed by name.
func (p *pkg) findFuncs(keep func(name string) bool) map[string]*ast.FuncDecl {
	out := map[string]*ast.FuncDecl{}
	for _, f := range p.files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && keep(fn.Name.Name) {
				out[fn.Name.Name] = fn
			}
		}
	}
	return out
}

// findValueSpec locates a top-level `var name = ...` and returns its first
// value expression together with the surrounding GenDecl doc, if any.
func (p *pkg) findValueSpec(name string) (*ast.ValueSpec, bool) {
	for _, f := range p.files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, id := range vs.Names {
					if id.Name == name {
						return vs, true
					}
				}
			}
		}
	}
	return nil, false
}

// commentByLine builds a map of line number -> trailing comment text for one
// file, so we can recover the per-element annotations in a composite literal
// (e.g. `8, // 0: CreateShipment - need entities`). Go's AST does not attach
// trailing comments to slice/array elements, so we match them by line.
func (p *pkg) commentByLine(file *ast.File) map[int]string {
	out := map[int]string{}
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			ln := p.line(c.Pos())
			out[ln] = cleanComment(c.Text)
		}
	}
	return out
}

// fileOf returns the *ast.File that contains pos.
func (p *pkg) fileOf(pos token.Pos) *ast.File {
	target := p.fset.Position(pos).Filename
	for path, f := range p.files {
		if path == target {
			return f
		}
	}
	return nil
}

// cleanComment strips comment markers and surrounding whitespace.
func cleanComment(s string) string {
	s = strings.TrimPrefix(s, "//")
	s = strings.TrimPrefix(s, "/*")
	s = strings.TrimSuffix(s, "*/")
	return strings.TrimSpace(s)
}

// docText flattens a doc comment group into a single trimmed string.
func docText(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	var b strings.Builder
	for i, c := range cg.List {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(cleanComment(c.Text))
	}
	return strings.TrimSpace(b.String())
}

// stringLit unquotes a string BasicLit, returning "" for anything else.
func stringLit(e ast.Expr) string {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return ""
	}
	s, err := strconv.Unquote(bl.Value)
	if err != nil {
		return ""
	}
	return s
}

// floatLit reads a numeric BasicLit (int or float) as a float64.
func floatLit(e ast.Expr) (float64, bool) {
	bl, ok := e.(*ast.BasicLit)
	if !ok || (bl.Kind != token.FLOAT && bl.Kind != token.INT) {
		return 0, false
	}
	v, err := strconv.ParseFloat(bl.Value, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// readField reads a string field from a struct composite literal using a field
// ref that is either positional ("0", "1") or a key name ("ID", "Label"). This
// unifies the two literal styles a catalogue might use:
//
//	{"transfer_fail", "Stripe transfer failure"}   // positional
//	{ID: "PAYMENT-1", Label: "NoNegativeBalances"} // keyed
func readField(lit *ast.CompositeLit, ref string) string {
	if idx, err := strconv.Atoi(ref); err == nil {
		// Positional: skip keyed elements (a mixed literal is invalid Go anyway).
		if idx >= 0 && idx < len(lit.Elts) {
			if _, keyed := lit.Elts[idx].(*ast.KeyValueExpr); !keyed {
				return stringLit(lit.Elts[idx])
			}
		}
		return ""
	}
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok && identName(kv.Key) == ref {
			return stringLit(kv.Value)
		}
	}
	return ""
}

// expandFuncPattern fills a checker-method template for one invariant id.
// Tokens: {ID} {Prefix} {prefix} {Title} {Num}. For "PAYMENT-11":
// {Prefix}=PAYMENT, {prefix}=payment, {Title}=Payment, {Num}=11, {ID}=PAYMENT-11.
func expandFuncPattern(pattern, id string) string {
	if pattern == "" {
		return ""
	}
	prefix, num := id, ""
	if dash := strings.IndexByte(id, '-'); dash >= 0 {
		prefix, num = id[:dash], id[dash+1:]
	}
	lower := strings.ToLower(prefix)
	title := lower
	if title != "" {
		title = strings.ToUpper(title[:1]) + title[1:]
	}
	r := strings.NewReplacer(
		"{ID}", id,
		"{Prefix}", prefix,
		"{prefix}", lower,
		"{Title}", title,
		"{Num}", num,
	)
	return r.Replace(pattern)
}

// elementLits returns the composite-literal elements of a slice/array literal,
// keeping only those that are themselves composite literals (the struct rows).
func elementLits(e ast.Expr) []*ast.CompositeLit {
	cl, ok := e.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var out []*ast.CompositeLit
	for _, elt := range cl.Elts {
		if inner, ok := elt.(*ast.CompositeLit); ok {
			out = append(out, inner)
		}
	}
	return out
}

// firstName returns the first identifier from a selector or ident expression,
// used to pull "FaultCrashAfterStripe" out of `e.recordFault(FaultCrashAfterStripe)`.
func identName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name
	}
	return ""
}
