package extract

import (
	"bytes"
	"go/ast"
	"go/printer"
	"go/token"
	"strings"
)

// extractState reads the configured state type and returns the mutable state the
// operations act on — the basis for the TLA+ VARIABLES the generator abstracts.
// A struct yields its fields; an interface yields its mutating methods (state is
// opaque behind a contract, so the methods are the best available signal). Nil
// when no state type is configured or the named type is absent: the TLA+ layer
// is optional and a missing state type is a gap, not a failure.
func extractState(p *pkg, cfg StateConfig) *StateModel {
	if cfg.TypeName == "" {
		return nil
	}
	ts, ok := p.findTypeSpec(cfg.TypeName)
	if !ok {
		return nil
	}

	sm := &StateModel{TypeName: cfg.TypeName, Loc: p.loc(ts.Pos())}
	switch t := ts.Type.(type) {
	case *ast.StructType:
		sm.Kind = "struct"
		sm.Variables = p.structVars(t)
	case *ast.InterfaceType:
		sm.Kind = "interface"
		sm.Variables = p.interfaceVars(t)
	default:
		return nil // a type alias or named primitive carries no state structure
	}
	return sm
}

// structVars lists a struct's fields as state variables, each with its rendered
// Go type and doc comment. Embedded structs are flattened: their fields are
// promoted into the parent so composition-built state (the common Go idiom)
// yields real variables instead of one opaque blob. Flattening is intra-module
// only — go/ast can resolve an embed solely when its type is declared in the
// parsed source. An embed it can't resolve (external/unparsed package, an
// interface, a cycle) is kept as a single opaque var with Unresolved set, so the
// gap is visible rather than silently swallowed.
func (p *pkg) structVars(st *ast.StructType) []StateVar {
	var out []StateVar
	p.collectStructVars(st, "", map[string]bool{}, map[string]bool{}, &out)
	return out
}

// collectStructVars walks st depth-first, promoting embedded-struct fields with
// shallower-wins shadowing (seen tracks names already emitted; a parent field
// shadows a same-named promoted one, matching Go's promotion rules well enough
// for a state model — genuine ambiguity is resolved deterministically by first
// occurrence). via is the dotted embed path into st; visiting guards cycles.
func (p *pkg) collectStructVars(st *ast.StructType, via string, seen, visiting map[string]bool, out *[]StateVar) {
	if st.Fields == nil {
		return
	}
	var embeds []*ast.Field // defer embeds so this level's named fields shadow them
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			embeds = append(embeds, f)
			continue
		}
		typ := p.renderNode(f.Type)
		doc := fieldDoc(f)
		for _, name := range f.Names {
			if seen[name.Name] {
				continue
			}
			seen[name.Name] = true
			*out = append(*out, StateVar{Name: name.Name, Type: typ, Doc: doc, Via: via, Loc: p.loc(name.Pos())})
		}
	}
	for _, f := range embeds {
		typ := p.renderNode(f.Type)
		// The promoted field name is the embedded type's base name: *Audit and
		// pkg.Audit both embed as "Audit" (pointer and qualifier stripped).
		name := lastSegment(strings.TrimPrefix(typ, "*"))
		inner, reason := p.resolveEmbeddedStruct(f.Type)
		if inner == nil || visiting[name] {
			if inner != nil { // a resolvable struct we've already entered: a cycle
				reason = "recursive embed"
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			*out = append(*out, StateVar{Name: name, Type: typ, Doc: fieldDoc(f), Via: via, Unresolved: reason, Loc: p.loc(f.Pos())})
			continue
		}
		visiting[name] = true
		p.collectStructVars(inner, joinVia(via, name), seen, visiting, out)
		delete(visiting, name)
	}
}

// resolveEmbeddedStruct resolves an embedded field's type to its *ast.StructType
// when that type is declared in the parsed source, so it can be flattened. It
// returns (nil, reason) when resolution isn't possible under go/ast alone: a
// qualified type from another package, an embedded interface, or a name with no
// declaration in the fileset. *T embeds are unwrapped to T first.
func (p *pkg) resolveEmbeddedStruct(e ast.Expr) (*ast.StructType, string) {
	switch t := e.(type) {
	case *ast.StarExpr:
		return p.resolveEmbeddedStruct(t.X)
	case *ast.SelectorExpr:
		// pkg.T — the qualifier names another package; matching by bare name
		// across packages would be unsound without go/types, so we don't.
		return nil, "external or cross-package embed"
	case *ast.Ident:
		ts, ok := p.findTypeSpec(t.Name)
		if !ok {
			return nil, "embedded type not declared in parsed source"
		}
		switch ts.Type.(type) {
		case *ast.StructType:
			return ts.Type.(*ast.StructType), ""
		case *ast.InterfaceType:
			return nil, "embedded interface (no fields to promote)"
		default:
			return nil, "embedded type is not a struct"
		}
	default:
		return nil, "unresolvable embedded type"
	}
}

// joinVia composes an embed path, e.g. "" + "Inner" -> "Inner", "A" + "B" -> "A.B".
func joinVia(via, name string) string {
	if via == "" {
		return name
	}
	return via + "." + name
}

// fieldDoc returns a field's doc comment, falling back to its trailing comment.
func fieldDoc(f *ast.Field) string {
	if doc := docText(f.Doc); doc != "" {
		return doc
	}
	return docText(f.Comment)
}

// interfaceVars lists an interface's methods as state variables: the method name
// and its rendered signature. Every declared method is surfaced faithfully;
// distinguishing mutators from read-only accessors is left to the generator.
// Embedded interfaces are skipped — their methods aren't declared here.
func (p *pkg) interfaceVars(it *ast.InterfaceType) []StateVar {
	var out []StateVar
	if it.Methods == nil {
		return out
	}
	for _, m := range it.Methods.List {
		fn, ok := m.Type.(*ast.FuncType)
		if !ok || len(m.Names) == 0 {
			continue // embedded interface; skip (its methods aren't declared here)
		}
		sig := "(" + p.renderFieldList(fn.Params) + ")"
		if res := p.renderFieldList(fn.Results); res != "" {
			if fn.Results != nil && len(fn.Results.List) > 1 {
				sig += " (" + res + ")"
			} else {
				sig += " " + res
			}
		}
		doc := docText(m.Doc)
		if doc == "" {
			doc = docText(m.Comment)
		}
		out = append(out, StateVar{Name: m.Names[0].Name, Type: sig, Doc: doc, Loc: p.loc(m.Pos())})
	}
	return out
}

// findTypeSpec locates a top-level `type Name ...` declaration in any parsed file.
func (p *pkg) findTypeSpec(name string) (*ast.TypeSpec, bool) {
	for _, f := range p.files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if ok && ts.Name.Name == name {
					return ts, true
				}
			}
		}
	}
	return nil, false
}

// renderNode prints an AST node back to Go source text using the package's
// fileset, so a field type or signature reads exactly as written.
func (p *pkg) renderNode(n ast.Node) string {
	var b bytes.Buffer
	if err := printer.Fprint(&b, p.fset, n); err != nil {
		return ""
	}
	return b.String()
}

// renderFieldList renders a parameter/result list as comma-separated Go text.
func (p *pkg) renderFieldList(fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	var parts []string
	for _, f := range fl.List {
		typ := p.renderNode(f.Type)
		if len(f.Names) == 0 {
			parts = append(parts, typ)
			continue
		}
		names := make([]string, len(f.Names))
		for i, n := range f.Names {
			names[i] = n.Name
		}
		parts = append(parts, strings.Join(names, ", ")+" "+typ)
	}
	return strings.Join(parts, ", ")
}
