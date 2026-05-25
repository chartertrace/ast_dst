package extract

import (
	"slices"
	"testing"
)

// TestExtractStructure runs the generic mode against the bundled fixture (which
// has functions, types, and package-level vars across two packages) and locks
// the structure mapping: funcs→operations, types→faults, vars/consts grouped by
// package→truths/invariants, and a real func→type reference edge.
func TestExtractStructure(t *testing.T) {
	t.Parallel()
	m, err := ExtractStructure(StructureConfig{Root: fixtureRoot})
	if err != nil {
		t.Fatalf("ExtractStructure: %v", err)
	}

	if m.Meta == nil || m.Meta.Mode != "structure" {
		t.Fatalf("expected structure meta, got %+v", m.Meta)
	}
	if len(m.Truths) != 2 { // catalog, engine
		t.Errorf("truths (packages) = %d, want 2", len(m.Truths))
	}
	if len(m.Operations) == 0 || len(m.Faults) == 0 || len(m.Invariants) == 0 {
		t.Fatalf("empty buckets: %d funcs, %d types, %d globals",
			len(m.Operations), len(m.Faults), len(m.Invariants))
	}

	// recordFault(f catalog.FaultType) must reference the FaultType type.
	var rf *Operation
	for i := range m.Operations {
		if m.Operations[i].Name == "recordFault" {
			rf = &m.Operations[i]
		}
	}
	if rf == nil {
		t.Fatal("recordFault function not extracted")
	}
	if !slices.Contains(rf.Faults, "FaultType") {
		t.Errorf("recordFault should reference FaultType, got %v", rf.Faults)
	}

	// That reference must surface as an op→fault edge to the qualified type id.
	wantEdge := Edge{From: opNodeID(rf.Index), To: "fault:catalog.FaultType", Kind: EdgeOpInjectsFault}
	if !slices.Contains(m.Edges, wantEdge) {
		t.Errorf("missing edge %+v", wantEdge)
	}

	// Every global is grouped under a real package truth.
	pkgs := map[string]bool{}
	for _, tr := range m.Truths {
		pkgs[tr.Name] = true
	}
	for _, inv := range m.Invariants {
		if !pkgs[inv.Truth] {
			t.Errorf("invariant %s has unknown package truth %q", inv.ID, inv.Truth)
		}
	}
}
