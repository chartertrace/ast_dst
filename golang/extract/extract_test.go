package extract

import (
	"fmt"
	"slices"
	"testing"
)

// fixtureRoot points at the self-contained synthetic codebase under testdata,
// relative to this test's directory (golang/extract). It uses the same
// catalogue/enum/trace names as the built-in preset, so the tests run
// DefaultConfig() against it with only Root and Packages adjusted.
const fixtureRoot = "testdata/sample"

func fixtureConfig() Config {
	cfg := DefaultConfig()
	cfg.Root = fixtureRoot
	cfg.Packages = []string{"catalog", "engine"} // the fixture has no "state" pkg
	return cfg
}

func mustExtract(t *testing.T) *Model {
	t.Helper()
	m, err := Extract(fixtureConfig())
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	return m
}

func TestCatalogueCounts(t *testing.T) {
	t.Parallel()
	m := mustExtract(t)

	cases := []struct {
		name string
		got  int
		want int
	}{
		{"truths", len(m.Truths), 2}, // ALPHA, BETA
		{"invariants", len(m.Invariants), 6},
		{"faults", len(m.Faults), 4},
		{"operations", len(m.Operations), 4},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestInvariantsCarryTruthAndLocation(t *testing.T) {
	t.Parallel()
	m := mustExtract(t)
	for _, inv := range m.Invariants {
		if inv.Truth == "" {
			t.Errorf("invariant %s has no truth", inv.ID)
		}
		if inv.Loc == nil {
			t.Errorf("invariant %s has no source location (CheckXxx not found)", inv.ID)
		}
	}
}

func TestOperationFaultEdgesComeFromCallSites(t *testing.T) {
	t.Parallel()
	m := mustExtract(t)

	// op 0 (CreateShipment) records FaultCreditRace; op 2 (Transfer) records
	// FaultTransferFail — both via recordFault(...) in the handler body.
	wantOp0 := false
	wantOp2 := false
	for _, op := range m.Operations {
		switch op.Index {
		case 0:
			wantOp0 = slices.Contains(op.Faults, "FaultCreditRace")
		case 2:
			wantOp2 = slices.Contains(op.Faults, "FaultTransferFail")
		}
	}
	if !wantOp0 {
		t.Error("op 0 should inject FaultCreditRace")
	}
	if !wantOp2 {
		t.Error("op 2 should inject FaultTransferFail")
	}
}

func TestEdgesReferenceRealNodes(t *testing.T) {
	t.Parallel()
	m := mustExtract(t)

	nodes := map[string]bool{}
	for _, tr := range m.Truths {
		nodes["truth:"+tr.Name] = true
	}
	for _, inv := range m.Invariants {
		nodes["inv:"+inv.ID] = true
	}
	for _, f := range m.Faults {
		nodes["fault:"+f.ID] = true
	}
	for _, op := range m.Operations {
		nodes[opNodeID(op.Index)] = true
	}
	for _, s := range m.Specs {
		nodes["spec:"+s.Name] = true
	}

	for _, e := range m.Edges {
		if !nodes[e.From] {
			t.Errorf("edge from unknown node %q", e.From)
		}
		if !nodes[e.To] {
			t.Errorf("edge to unknown node %q", e.To)
		}
	}
}

func opNodeID(i int) string {
	return fmt.Sprintf("op:%d", i)
}
