package extract

import (
	"go/ast"
	"go/token"
	"slices"
)

// extractFaults builds the fault list from whichever sources the config names.
// The catalogue slice (StructSliceVar) gives id + label + order; an optional
// enum type supplies Go constant names and doc comments; an optional name map
// links the two. A codebase with only an enum (no catalogue) still works.
func extractFaults(p *pkg, cfg FaultConfig) []Fault {
	idToLabel, order := faultCatalogue(p, cfg)
	enumToID := faultNameMap(p, cfg)
	enumDocs, enumLocs, enumOrder := faultEnumDocs(p, cfg)

	idToEnum := map[string]string{}
	for enum, id := range enumToID {
		idToEnum[id] = enum
	}

	// Prefer catalogue order; fall back to enum declaration order.
	if len(order) == 0 {
		for _, enum := range enumOrder {
			if id := enumToID[enum]; id != "" {
				order = append(order, id)
			} else {
				// Enum-only codebase (no name map): the enum constant IS the id.
				// Bind the identity so the loop below resolves enumDocs/enumLocs —
				// without this the doc, location, and Enum field are silently
				// dropped for the very enum the catalogue lookup just found.
				order = append(order, enum)
				idToEnum[enum] = enum
			}
		}
	}

	out := make([]Fault, 0, len(order))
	for _, id := range order {
		enum := idToEnum[id]
		f := Fault{ID: id, Enum: enum, Label: idToLabel[id], Doc: enumDocs[enum]}
		if loc, ok := enumLocs[enum]; ok {
			f.Loc = loc
		}
		out = append(out, f)
	}
	return out
}

// faultCatalogue reads `var <StructSliceVar> = []T{ {id, label}, ... }`,
// returning id->label and the id order. Empty when no slice var is configured.
func faultCatalogue(p *pkg, cfg FaultConfig) (map[string]string, []string) {
	labels := map[string]string{}
	var order []string
	if cfg.StructSliceVar == "" {
		return labels, order
	}
	vs, ok := p.findValueSpec(cfg.StructSliceVar)
	if !ok || len(vs.Values) == 0 {
		return labels, order
	}
	for _, lit := range elementLits(vs.Values[0]) {
		id := readField(lit, cfg.IDField)
		if id == "" {
			continue
		}
		labels[id] = readField(lit, cfg.LabelField)
		order = append(order, id)
	}
	return labels, order
}

// faultNameMap reads `var <NameMapVar> = map[Enum]string{ Enum: "id" }`.
func faultNameMap(p *pkg, cfg FaultConfig) map[string]string {
	out := map[string]string{}
	if cfg.NameMapVar == "" {
		return out
	}
	vs, ok := p.findValueSpec(cfg.NameMapVar)
	if !ok || len(vs.Values) == 0 {
		return out
	}
	cl, ok := vs.Values[0].(*ast.CompositeLit)
	if !ok {
		return out
	}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		enum := identName(kv.Key)
		id := stringLit(kv.Value)
		if enum != "" && id != "" && !slices.Contains(cfg.EnumSkip, enum) {
			out[enum] = id
		}
	}
	return out
}

// faultEnumDocs walks every `const ( ... )` block that declares the configured
// enum type and returns each constant's doc comment, location, and order.
func faultEnumDocs(p *pkg, cfg FaultConfig) (docs map[string]string, locs map[string]*Loc, order []string) {
	docs = map[string]string{}
	locs = map[string]*Loc{}
	if cfg.EnumType == "" {
		return docs, locs, order
	}
	for _, f := range p.files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST || !constBlockHasType(gd, cfg.EnumType) {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || len(vs.Names) == 0 {
					continue
				}
				name := vs.Names[0].Name
				if slices.Contains(cfg.EnumSkip, name) {
					continue
				}
				doc := docText(vs.Comment)
				if doc == "" {
					doc = docText(vs.Doc)
				}
				if doc != "" {
					docs[name] = doc
				}
				locs[name] = p.loc(vs.Pos())
				order = append(order, name)
			}
		}
	}
	return docs, locs, order
}

// constBlockHasType reports whether any spec in a const block declares typ —
// in an iota block only the first spec carries the type, so we treat the whole
// block as that type.
func constBlockHasType(gd *ast.GenDecl, typ string) bool {
	for _, spec := range gd.Specs {
		if vs, ok := spec.(*ast.ValueSpec); ok {
			if id, ok := vs.Type.(*ast.Ident); ok && id.Name == typ {
				return true
			}
		}
	}
	return false
}
