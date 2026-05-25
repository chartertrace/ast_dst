package extract

import "testing"

// TestEnumOnlyFaultsCarryDocAndLoc covers the enum-only fault path: a codebase
// with a fault enum but no catalogue slice and no name map. Regression guard for
// the bug where, without a name map, each fault's Enum/Doc/Loc were silently
// dropped even though faultEnumDocs had collected them. The sim fixture cannot
// catch this — it always supplies faultNames — so this uses its own fixture.
func TestEnumOnlyFaultsCarryDocAndLoc(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Root:   "testdata/enumonly", // sibling of sample, so sample scans are unaffected
		Faults: FaultConfig{EnumType: "Failure"},
		// Invariants/Operations/TLA/State left empty: this test is about faults.
	}
	m, err := Extract(cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(m.Faults) != 2 {
		t.Fatalf("faults = %d, want 2 (FailTimeout, FailCorrupt)", len(m.Faults))
	}
	for _, f := range m.Faults {
		if f.Enum == "" {
			t.Errorf("fault %q lost its Enum field (enum-only path dropped it)", f.ID)
		}
		if f.Enum != f.ID {
			t.Errorf("fault %q: with no name map, Enum should equal ID, got Enum=%q", f.ID, f.Enum)
		}
		if f.Doc == "" {
			t.Errorf("fault %q lost its doc comment", f.ID)
		}
		if f.Loc == nil {
			t.Errorf("fault %q lost its source location", f.ID)
		}
	}
}
