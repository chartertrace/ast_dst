package extract

import (
	"go/ast"
	"sort"
	"strings"
)

// extractOperations reconstructs the weighted operation table from config:
//   - TableVar       -> index -> handler name, in dispatch order
//   - WeightsFunc    -> the [N]float64 weight literal assigned to WeightsVar
//   - each handler   -> the fault identities passed to the TraceCall it makes
func extractOperations(p *pkg, cfg OperationConfig) []Operation {
	handlers := opTableOrder(p, cfg.TableVar)
	weights, notes := opWeights(p, cfg)
	handlerTargets := traceCalls(p, cfg)
	prefix := cfg.handlerPrefix()
	handlerDecls := p.findFuncs(func(n string) bool { return strings.HasPrefix(n, prefix) })

	var total float64
	for _, w := range weights {
		total += w
	}

	out := make([]Operation, 0, len(handlers))
	for idx, handler := range handlers {
		op := Operation{
			Index:   idx,
			Handler: handler,
			Name:    strings.TrimPrefix(handler, cfg.NamePrefix),
			Faults:  handlerTargets[handler],
		}
		if idx < len(weights) {
			op.Weight = weights[idx]
			if total > 0 {
				op.Share = weights[idx] / total
			}
		}
		// Keep a note only when it adds rationale beyond echoing the op name.
		if n, ok := notes[idx]; ok && !strings.EqualFold(n, op.Name) {
			op.Note = n
		}
		if fn, ok := handlerDecls[handler]; ok {
			op.Loc = p.loc(fn.Pos())
		}
		if op.Faults == nil {
			op.Faults = []string{}
		}
		out = append(out, op)
	}
	return out
}

// opTableOrder reads `var <tableVar> = [N]func(...){ handlerA, ... }` and
// returns the handler names in index order.
func opTableOrder(p *pkg, tableVar string) []string {
	if tableVar == "" {
		return nil
	}
	vs, ok := p.findValueSpec(tableVar)
	if !ok || len(vs.Values) == 0 {
		return nil
	}
	cl, ok := vs.Values[0].(*ast.CompositeLit)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(cl.Elts))
	for _, elt := range cl.Elts {
		if name := identName(elt); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// opWeights finds the `<WeightsVar> = [N]float64{...}` assignment inside
// WeightsFunc and returns the weights plus the trailing comment on each line.
func opWeights(p *pkg, cfg OperationConfig) (weights []float64, notes map[int]string) {
	notes = map[int]string{}
	if cfg.WeightsFunc == "" || cfg.WeightsVar == "" {
		return nil, notes
	}
	fn := p.findFuncs(func(n string) bool { return n == cfg.WeightsFunc })[cfg.WeightsFunc]
	if fn == nil || fn.Body == nil {
		return nil, notes
	}
	file := p.fileOf(fn.Pos())
	var byLine map[int]string
	if file != nil {
		byLine = p.commentByLine(file)
	}

	var lit *ast.CompositeLit
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			return true
		}
		if identName(as.Lhs[0]) != cfg.WeightsVar {
			return true
		}
		if cl, ok := as.Rhs[0].(*ast.CompositeLit); ok {
			lit = cl
		}
		return false
	})
	if lit == nil {
		return nil, notes
	}

	for i, elt := range lit.Elts {
		if v, ok := floatLit(elt); ok {
			weights = append(weights, v)
		} else {
			weights = append(weights, 0)
		}
		if byLine != nil {
			if c, ok := byLine[p.line(elt.Pos())]; ok {
				notes[i] = cleanWeightNote(c)
			}
		}
	}
	return weights, notes
}

// cleanWeightNote turns "0: CreateShipment - need entities to work with" into
// "need entities to work with" (drops a leading "<n>:" index and a handler echo).
func cleanWeightNote(s string) string {
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.Index(s, " - "); i >= 0 {
		s = s[i+3:]
	}
	return strings.TrimSpace(s)
}

// traceCalls walks every handler-prefixed function body and collects the
// identifiers passed to the configured TraceCall at TraceArgIndex. This is the
// operation->fault edge, derived from real instrumentation call sites — works
// for any trace API (recordFault, span.AddEvent, tracer.Record, …) since we
// match on the called function/method's final name.
func traceCalls(p *pkg, cfg OperationConfig) map[string][]string {
	out := map[string][]string{}
	if cfg.TraceCall == "" {
		return out
	}
	want := lastSegment(cfg.TraceCall)
	prefix := cfg.handlerPrefix()
	fns := p.findFuncs(func(n string) bool { return strings.HasPrefix(n, prefix) })
	for name, fn := range fns {
		if fn.Body == nil {
			continue
		}
		seen := map[string]bool{}
		var targets []string
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || calleeName(call.Fun) != want {
				return true
			}
			if cfg.TraceArgIndex >= len(call.Args) {
				return true
			}
			if t := identName(call.Args[cfg.TraceArgIndex]); t != "" && !seen[t] {
				seen[t] = true
				targets = append(targets, t)
			}
			return true
		})
		if len(targets) > 0 {
			sort.Strings(targets)
			out[name] = targets
		}
	}
	return out
}

// calleeName returns the final name of a call target: "recordFault" for a plain
// ident, or the method name for a selector (e.g. "AddEvent" in span.AddEvent).
func calleeName(fun ast.Expr) string {
	switch v := fun.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name
	}
	return ""
}

// lastSegment returns the part of a dotted config value after the final dot, so
// "span.AddEvent" matches a call whose method name is "AddEvent".
func lastSegment(s string) string {
	if i := strings.LastIndexByte(s, '.'); i >= 0 {
		return s[i+1:]
	}
	return s
}
