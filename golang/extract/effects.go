package extract

import (
	"go/ast"
	"go/token"
	"sort"
	"strings"
)

// extractEffects finds, for each operation handler, how it mutates the state
// struct's fields — the basis for the TLA+ transitions. It is deliberately
// syntactic (no go/types): a write is an assignment or ++/-- whose left-hand
// side, peeled of indexing and dereferencing, ends in `.Field` where Field is a
// known state field. When the statement form is a simple, deterministic shape
// (`f++`, `f += 3`, `f = 2`, `f = true`, `f = "x"`) the value form is recovered
// so the generator can emit `f' = f + 1` rather than a nondeterministic bound;
// otherwise only the field name is recorded (the generator falls back).
//
// A field written in several conflicting ways within one handler is reported as
// a plain write with no recovered form, so the generator never picks one
// arbitrarily. State behind an interface exposes no fields, so its handlers
// yield nothing here.
func extractEffects(p *pkg, cfg OperationConfig, stateFields map[string]bool) map[string][]FieldEffect {
	out := map[string][]FieldEffect{}
	if len(stateFields) == 0 {
		return out
	}
	prefix := cfg.handlerPrefix()
	for name, fn := range p.findFuncs(func(n string) bool { return strings.HasPrefix(n, prefix) }) {
		if fn.Body == nil {
			continue
		}
		// Collect every recovered form per field; resolve conflicts afterwards.
		forms := map[string][]FieldEffect{}
		note := func(field string, eff *FieldEffect) {
			if field == "" || !stateFields[field] {
				return
			}
			if eff == nil {
				forms[field] = append(forms[field], FieldEffect{Field: field, Op: ""})
				return
			}
			eff.Field = field
			forms[field] = append(forms[field], *eff)
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch s := n.(type) {
			case *ast.IncDecStmt:
				op := "inc"
				if s.Tok == token.DEC {
					op = "dec"
				}
				note(writtenField(s.X), &FieldEffect{Op: op})
			case *ast.AssignStmt:
				// Only single, scalar field assignments carry a clean value form;
				// multi-assign or indexed targets record a bare write.
				if len(s.Lhs) == 1 && len(s.Rhs) == 1 {
					field := writtenField(s.Lhs[0])
					if _, indexed := s.Lhs[0].(*ast.IndexExpr); indexed {
						note(field, nil) // map/slice element write: form not modellable here
					} else {
						note(field, assignEffect(s))
					}
				} else {
					for _, lhs := range s.Lhs {
						note(writtenField(lhs), nil)
					}
				}
			}
			return true
		})
		if len(forms) == 0 {
			continue
		}
		out[name] = resolveEffects(forms)
	}
	return out
}

// assignEffect maps a single `lhs op= rhs` assignment to a recovered FieldEffect,
// or nil when the form isn't one we model deterministically.
func assignEffect(s *ast.AssignStmt) *FieldEffect {
	switch s.Tok {
	case token.ADD_ASSIGN: // f += n
		if v, ok := intLiteral(s.Rhs[0]); ok {
			return &FieldEffect{Op: "add", Value: v}
		}
	case token.SUB_ASSIGN: // f -= n
		if v, ok := intLiteral(s.Rhs[0]); ok {
			return &FieldEffect{Op: "sub", Value: v}
		}
	case token.ASSIGN: // f = <literal>
		if v, ok := intLiteral(s.Rhs[0]); ok {
			return &FieldEffect{Op: "setNum", Value: v}
		}
		if v, ok := boolLiteral(s.Rhs[0]); ok {
			return &FieldEffect{Op: "setBool", Value: v}
		}
		if lit, ok := s.Rhs[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			return &FieldEffect{Op: "setStr", Value: lit.Value} // already quoted
		}
	}
	return nil // anything else: form not recovered
}

// resolveEffects collapses each field's recorded forms into one FieldEffect: the
// single distinct form if they all agree, otherwise a bare write (Op "") so the
// generator falls back to a nondeterministic bound rather than guess.
func resolveEffects(forms map[string][]FieldEffect) []FieldEffect {
	out := make([]FieldEffect, 0, len(forms))
	for field, fs := range forms {
		first := fs[0]
		uniform := true
		for _, f := range fs[1:] {
			if f.Op != first.Op || f.Value != first.Value {
				uniform = false
				break
			}
		}
		if uniform {
			out = append(out, first)
		} else {
			out = append(out, FieldEffect{Field: field}) // conflicting writes → no form
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

// writtenField peels indexing (`m[k]`), slicing, dereferencing, and parens off an
// assignment target and returns the trailing selector field name, or "" if the
// target is not a field selection (e.g. a bare local variable).
func writtenField(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.SelectorExpr:
		return v.Sel.Name
	case *ast.IndexExpr:
		return writtenField(v.X)
	case *ast.IndexListExpr:
		return writtenField(v.X)
	case *ast.StarExpr:
		return writtenField(v.X)
	case *ast.ParenExpr:
		return writtenField(v.X)
	case *ast.SliceExpr:
		return writtenField(v.X)
	default:
		return ""
	}
}

// intLiteral returns the text of an integer literal expression (rejecting
// negatives and non-literals), used for `+= n` / `= n` recovery.
func intLiteral(e ast.Expr) (string, bool) {
	if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.INT {
		return lit.Value, true
	}
	return "", false
}

// boolLiteral returns "TRUE"/"FALSE" for the identifiers true/false.
func boolLiteral(e ast.Expr) (string, bool) {
	if id, ok := e.(*ast.Ident); ok {
		switch id.Name {
		case "true":
			return "TRUE", true
		case "false":
			return "FALSE", true
		}
	}
	return "", false
}

// stateFieldSet returns the struct field names of the state model, for write
// matching. Empty for interface state (no visible fields) or no state.
func stateFieldSet(sm *StateModel) map[string]bool {
	if sm == nil || sm.Kind != "struct" {
		return nil
	}
	out := make(map[string]bool, len(sm.Variables))
	for _, v := range sm.Variables {
		out[v.Name] = true
	}
	return out
}
