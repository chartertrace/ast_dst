package extract

import (
	"go/ast"
	"sort"
	"strings"
)

// extractWrites finds, for each operation handler, the state-struct fields it
// assigns to — the write set that becomes the operation's TLA+ transition. It is
// deliberately syntactic (no go/types): a write is an assignment, ++/--, or
// range-assign whose left-hand side, peeled of indexing and dereferencing, ends
// in a selector `.Field` where Field is one of the known state fields. This
// catches `s.Clock++`, `s.Shipments[id] = "x"`, `e.state.Balances[d] += n`, etc.
//
// Matching by field name (rather than resolving the receiver's type) keeps the
// extractor dependency-free and works whether the handler holds the state struct
// directly or reaches it through another value. The cost is the usual heuristic
// risk: a same-named field on an unrelated struct would also match. State behind
// an interface exposes no fields, so its handlers yield empty write sets.
func extractWrites(p *pkg, cfg OperationConfig, stateFields map[string]bool) map[string][]string {
	out := map[string][]string{}
	if len(stateFields) == 0 {
		return out
	}
	prefix := cfg.handlerPrefix()
	for name, fn := range p.findFuncs(func(n string) bool { return strings.HasPrefix(n, prefix) }) {
		if fn.Body == nil {
			continue
		}
		seen := map[string]bool{}
		record := func(field string) {
			if field != "" && stateFields[field] && !seen[field] {
				seen[field] = true
			}
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch s := n.(type) {
			case *ast.AssignStmt:
				for _, lhs := range s.Lhs {
					record(writtenField(lhs))
				}
			case *ast.IncDecStmt:
				record(writtenField(s.X))
			}
			return true
		})
		if len(seen) > 0 {
			fields := make([]string, 0, len(seen))
			for f := range seen {
				fields = append(fields, f)
			}
			sort.Strings(fields)
			out[name] = fields
		}
	}
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
