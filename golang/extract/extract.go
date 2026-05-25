package extract

import (
	"fmt"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Extract parses the codebase rooted at cfg.Root (the configured packages, or
// the whole tree) and returns the domain model — operations, faults, invariants
// grouped by truth, and the edges the source encodes. All names it looks for
// come from cfg, so the same extractor serves any sim-shaped codebase.
func Extract(cfg Config) (*Model, error) {
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

	faults := extractFaults(p, cfg.Faults)
	invariants, truths := extractInvariants(p, cfg.Invariants)
	operations := extractOperations(p, cfg.Operations)
	state := extractState(p, cfg.State)

	// Attach each operation's state write set (the basis for its TLA+ transition).
	if writes := extractWrites(p, cfg.Operations, stateFieldSet(state)); len(writes) > 0 {
		for i := range operations {
			operations[i].Writes = writes[operations[i].Handler]
		}
	}

	m := &Model{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Source: Source{
			SimPath:    cfg.Root,
			FilesRead:  len(p.files),
			ToolModule: "astdst",
		},
		Truths:     truths,
		Invariants: invariants,
		Faults:     faults,
		Operations: operations,
		State:      state,
	}

	// Bind the runtime invariant checkers to the formal TLA+ spec they validate,
	// if one is configured. Skipped (and the model carries no specs) otherwise.
	if cfg.TLA.SpecDir != "" {
		specDir := cfg.TLA.SpecDir
		if !filepath.IsAbs(specDir) {
			specDir = filepath.Join(absRoot, specDir)
		}
		specs, err := parseSpecs(specDir, cfg.TLA.CfgFile, absRoot)
		if err != nil {
			return nil, fmt.Errorf("parse TLA specs in %s: %w", cfg.TLA.SpecDir, err)
		}
		m.Specs = specs
		attachSpecs(m, cfg.TLA)
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
	m.Stats.SpecsTotal = len(m.Specs)
	for _, inv := range m.Invariants {
		switch inv.SpecStatus {
		case SpecValidated:
			m.Stats.Validated++
		case SpecUnchecked:
			m.Stats.UncheckedSpec++
		case SpecUnspecified:
			m.Stats.Unspecified++
		}
	}
	return m, nil
}

// attachSpecs binds each Go invariant to the spec invariant it validates and
// classifies the coverage. Matching is by label (the sim's convention: a Go
// invariant's Label equals the TLA+ operator name), so a Go checker with no
// counterpart — PAYMENT-9..14, the whole ECONOMICS truth — is "unspecified":
// the implementation validates more than the formal spec covers, shown as such.
func attachSpecs(m *Model, cfg TLAConfig) {
	byName := make(map[string]SpecInvariant, len(m.Specs))
	for _, s := range m.Specs {
		byName[s.Name] = s
	}
	for i := range m.Invariants {
		inv := &m.Invariants[i]
		var spec *SpecInvariant
		if cfg.MatchByLabel && inv.Label != "" {
			if s, ok := byName[inv.Label]; ok {
				s := s
				spec = &s
			}
		}
		if spec == nil {
			inv.SpecStatus = SpecUnspecified
			continue
		}
		inv.Spec = &SpecRef{
			Name:      spec.Name,
			SpecFile:  spec.SpecFile,
			Checked:   spec.Checked,
			Loc:       spec.Loc,
			Generated: spec.Generated,
			Verified:  spec.Verified,
		}
		if spec.Checked {
			inv.SpecStatus = SpecValidated
		} else {
			inv.SpecStatus = SpecUnchecked
		}
	}
}

// buildEdges derives every cross-link in the model purely from extracted data:
//   - truth -> invariant   (id-prefix grouping)
//   - op    -> fault        (trace-call sites)
//   - doc   -> invariant    (an invariant/fault doc that names another invariant id)
func buildEdges(m *Model) []Edge {
	var edges []Edge

	for _, inv := range m.Invariants {
		if inv.Truth == "" {
			continue
		}
		edges = append(edges, Edge{
			From: "truth:" + inv.Truth,
			To:   "inv:" + inv.ID,
			Kind: EdgeTruthHasInvariant,
		})
	}

	// Operation handlers reference faults by their Go enum name; map back to id.
	enumToID := map[string]string{}
	for _, f := range m.Faults {
		if f.Enum != "" {
			enumToID[f.Enum] = f.ID
		}
	}
	for _, op := range m.Operations {
		for _, target := range op.Faults {
			id, ok := enumToID[target]
			if !ok {
				// Trace arg may already be the fault id rather than an enum.
				if _, isFault := faultByID(m, target); isFault {
					id = target
				} else {
					continue
				}
			}
			edges = append(edges, Edge{
				From: fmt.Sprintf("op:%d", op.Index),
				To:   "fault:" + id,
				Kind: EdgeOpInjectsFault,
			})
		}
	}

	// Mention edges: a fault or invariant doc that names a known invariant id.
	ids := make([]string, 0, len(m.Invariants))
	for _, inv := range m.Invariants {
		ids = append(ids, inv.ID)
	}
	for _, f := range m.Faults {
		for _, to := range mentionedIDs(f.Doc, ids) {
			edges = append(edges, Edge{From: "fault:" + f.ID, To: "inv:" + to, Kind: EdgeMentionsInvariant})
		}
	}
	for _, inv := range m.Invariants {
		for _, to := range mentionedIDs(inv.Doc, ids) {
			if to == inv.ID {
				continue
			}
			edges = append(edges, Edge{From: "inv:" + inv.ID, To: "inv:" + to, Kind: EdgeMentionsInvariant})
		}
	}

	// Validation edges: a Go invariant -> the TLA+ spec invariant it enforces.
	for _, inv := range m.Invariants {
		if inv.Spec != nil {
			edges = append(edges, Edge{From: "inv:" + inv.ID, To: "spec:" + inv.Spec.Name, Kind: EdgeValidates})
		}
	}

	dedupeEdges(&edges)
	return edges
}

// MarkGenerated stamps machine-generation provenance onto every spec in the
// model and onto the invariant refs that bind to them. Called after re-extracting
// a model whose TLA+ spec was produced by package gen, so the viewer can badge
// generated/verified specs. verified must be the TLC outcome — never set true for
// a spec TLC did not check clean.
func MarkGenerated(m *Model, verified bool, tlcStates, tlcDepth int) {
	for i := range m.Specs {
		m.Specs[i].Generated = true
		m.Specs[i].Verified = verified
		m.Specs[i].TLCStates = tlcStates
		m.Specs[i].TLCDepth = tlcDepth
	}
	for i := range m.Invariants {
		if ref := m.Invariants[i].Spec; ref != nil {
			ref.Generated = true
			ref.Verified = verified
		}
	}
}

func faultByID(m *Model, id string) (Fault, bool) {
	for _, f := range m.Faults {
		if f.ID == id {
			return f, true
		}
	}
	return Fault{}, false
}

// mentionedIDs returns the invariant IDs that appear verbatim in doc as whole
// tokens — "PAYMENT-1" must not match inside "PAYMENT-10".
func mentionedIDs(doc string, ids []string) []string {
	if doc == "" {
		return nil
	}
	var out []string
	for _, id := range ids {
		if containsToken(doc, id) {
			out = append(out, id)
		}
	}
	return out
}

// containsToken reports whether tok appears in s not immediately followed by a
// digit or hyphen (so an ID isn't matched as the prefix of a longer ID).
func containsToken(s, tok string) bool {
	from := 0
	for {
		i := strings.Index(s[from:], tok)
		if i < 0 {
			return false
		}
		end := from + i + len(tok)
		if end >= len(s) || (s[end] != '-' && (s[end] < '0' || s[end] > '9')) {
			return true
		}
		from = end
	}
}

func dedupeEdges(edges *[]Edge) {
	seen := map[Edge]bool{}
	out := (*edges)[:0]
	for _, e := range *edges {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	*edges = out
}

func totalWeight(ops []Operation) float64 {
	var t float64
	for _, op := range ops {
		t += op.Weight
	}
	return t
}
