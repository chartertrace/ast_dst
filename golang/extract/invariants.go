package extract

import (
	"go/ast"
	"strings"
)

// extractInvariants reads the configured invariant catalogue slice and, when a
// DocFuncPattern is given, enriches each entry with the doc comment + location
// of its checker method (e.g. "PAYMENT-11" -> CheckPayment11). Truths are the
// id prefixes when GroupFromIDPrefix is set.
func extractInvariants(p *pkg, cfg InvariantConfig) ([]Invariant, []Truth) {
	if cfg.StructSliceVar == "" {
		return nil, nil
	}
	vs, ok := p.findValueSpec(cfg.StructSliceVar)
	if !ok || len(vs.Values) == 0 {
		return nil, nil
	}

	var checkers map[string]*ast.FuncDecl
	if cfg.DocFuncPattern != "" {
		checkers = p.findFuncs(func(string) bool { return true })
	}

	var out []Invariant
	truthCount := map[string]int{}
	var truthOrder []string

	for _, lit := range elementLits(vs.Values[0]) {
		id := readField(lit, cfg.IDField)
		if id == "" {
			continue
		}
		inv := Invariant{ID: id, Label: readField(lit, cfg.LabelField)}

		if cfg.GroupFromIDPrefix {
			truth := id
			if j := strings.IndexByte(id, '-'); j > 0 {
				truth = id[:j]
			}
			inv.Truth = truth
			if truthCount[truth] == 0 {
				truthOrder = append(truthOrder, truth)
			}
			truthCount[truth]++
		}

		if checkers != nil {
			if fn, ok := checkers[expandFuncPattern(cfg.DocFuncPattern, id)]; ok {
				inv.Doc = docText(fn.Doc)
				inv.Loc = p.loc(fn.Pos())
			}
		}
		out = append(out, inv)
	}

	truths := make([]Truth, 0, len(truthOrder))
	for _, name := range truthOrder {
		truths = append(truths, Truth{Name: name, Count: truthCount[name]})
	}
	return out, truths
}
