package extract

import (
	"os"
	"path/filepath"
	"testing"
)

// --- Parser unit tests (no sim checkout required) ---------------------------

const sampleTLA = `---- MODULE Sample ----

\* INV-1: balances never go negative
NoNegativeBalances ==
    \A d \in Drivers: balance[d] >= 0

\* a helper, not an invariant
NextId == Cardinality(xs) + 1

\* the most important property
NoUnpaidCompletedWork ==
    \A s \in Shipments:
        state[s] = "completed" =>
            \E e \in ledger: e.shipment = s

Invariants ==
    /\ TypeOK
    /\ NoNegativeBalances
    /\ NoUnpaidCompletedWork

====
`

func TestParseSpecFileExtractsInvariants(t *testing.T) {
	t.Parallel()
	defs, declared := parseSpecFile(sampleTLA, "Sample.tla", "Sample.tla")

	// Aggregator conjuncts are the declared invariants; helpers are not.
	for _, want := range []string{"TypeOK", "NoNegativeBalances", "NoUnpaidCompletedWork"} {
		if !declared[want] {
			t.Errorf("expected %q to be declared as an invariant", want)
		}
	}
	if declared["NextId"] {
		t.Error("NextId is a helper, not an invariant")
	}

	// Definition bodies are captured verbatim (multi-line included).
	d, ok := defs["NoNegativeBalances"]
	if !ok {
		t.Fatal("NoNegativeBalances definition not parsed")
	}
	if d.predicate != `\A d \in Drivers: balance[d] >= 0` {
		t.Errorf("predicate = %q", d.predicate)
	}
	if d.doc != "INV-1: balances never go negative" {
		t.Errorf("doc = %q", d.doc)
	}
	if d.loc == nil || d.loc.Line != 4 {
		t.Errorf("loc = %+v, want line 4", d.loc)
	}
	// Multi-line body, dedented: starts at the \A quantifier, keeps relative
	// indentation of nested lines.
	wantMulti := "\\A s \\in Shipments:\n    state[s] = \"completed\" =>\n        \\E e \\in ledger: e.shipment = s"
	if multi := defs["NoUnpaidCompletedWork"].predicate; multi != wantMulti {
		t.Errorf("multi-line predicate = %q, want %q", multi, wantMulti)
	}
}

func TestParseCfgInvariants(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "X.cfg")
	body := "\\* comment\nSPECIFICATION Spec\nINVARIANT NoNegativeBalances\nINVARIANT TypeOK\nINVARIANTS A B C\nCHECK_DEADLOCK FALSE\n"
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parseCfgInvariants(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"NoNegativeBalances", "TypeOK", "A", "B", "C"} {
		if !got[want] {
			t.Errorf("expected %q in checked set", want)
		}
	}
	if got["SPECIFICATION"] || got["Spec"] {
		t.Error("non-INVARIANT lines leaked into the checked set")
	}
}

func TestParseCfgInvariantsMissingFile(t *testing.T) {
	t.Parallel()
	got, err := parseCfgInvariants(filepath.Join(t.TempDir(), "nope.cfg"))
	if err != nil {
		t.Fatalf("missing .cfg should not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("missing .cfg should yield empty set, got %v", got)
	}
}

// --- Integration tests (run against the testdata fixture) -------------------
//
// The fixture's TLA+ spec (testdata/docs/tla/PayoutFlow.tla + .cfg) declares two
// invariants by operator name — NoNegativeBalance and ConservationOfValue — and
// model-checks only the former. The Go catalogue's ALPHA-1/ALPHA-2 labels match
// those names, so ALPHA-1 reads "validated", ALPHA-2 "unchecked-spec", and the
// remaining checkers (no TLA+ counterpart) "unspecified".

// TestSpecCoverageIsExhaustive locks the invariant that every Go checker lands
// in exactly one coverage bucket — so the three counts always sum to the total.
func TestSpecCoverageIsExhaustive(t *testing.T) {
	t.Parallel()
	m := mustExtract(t)
	if m.Stats.SpecsTotal == 0 {
		t.Fatal("no specs extracted; expected the TLA+ layer to be active")
	}
	sum := m.Stats.Validated + m.Stats.UncheckedSpec + m.Stats.Unspecified
	if sum != m.Stats.Invariants {
		t.Errorf("coverage buckets sum to %d, want %d (every invariant must be classified)", sum, m.Stats.Invariants)
	}
	for _, inv := range m.Invariants {
		switch inv.SpecStatus {
		case SpecValidated, SpecUnchecked, SpecUnspecified:
		default:
			t.Errorf("invariant %s has unclassified specStatus %q", inv.ID, inv.SpecStatus)
		}
	}
}

// TestModelCheckedInvariantsAreValidated proves a checked TLA+ invariant with a
// live Go checker reads "validated" and carries its formal spec reference.
func TestModelCheckedInvariantsAreValidated(t *testing.T) {
	t.Parallel()
	m := mustExtract(t)
	byID := map[string]Invariant{}
	for _, inv := range m.Invariants {
		byID[inv.ID] = inv
	}
	// ALPHA-1 = NoNegativeBalance, the one invariant model-checked in PayoutFlow.cfg.
	inv, ok := byID["ALPHA-1"]
	if !ok {
		t.Fatal("ALPHA-1 missing")
	}
	if inv.SpecStatus != SpecValidated {
		t.Errorf("ALPHA-1 specStatus = %q, want %q", inv.SpecStatus, SpecValidated)
	}
	if inv.Spec == nil || inv.Spec.Name != "NoNegativeBalance" || !inv.Spec.Checked {
		t.Errorf("ALPHA-1 spec ref = %+v, want NoNegativeBalance checked", inv.Spec)
	}
}

// TestUncheckedSpecInvariant proves an invariant declared in TLA+ but absent
// from the .cfg checked set reads "unchecked-spec" and still links its spec.
func TestUncheckedSpecInvariant(t *testing.T) {
	t.Parallel()
	m := mustExtract(t)
	byID := map[string]Invariant{}
	for _, inv := range m.Invariants {
		byID[inv.ID] = inv
	}
	// ALPHA-2 = ConservationOfValue: defined in the spec, not in the .cfg.
	inv, ok := byID["ALPHA-2"]
	if !ok {
		t.Fatal("ALPHA-2 missing")
	}
	if inv.SpecStatus != SpecUnchecked {
		t.Errorf("ALPHA-2 specStatus = %q, want %q", inv.SpecStatus, SpecUnchecked)
	}
	if inv.Spec == nil || inv.Spec.Name != "ConservationOfValue" || inv.Spec.Checked {
		t.Errorf("ALPHA-2 spec ref = %+v, want ConservationOfValue unchecked", inv.Spec)
	}
}

// TestUnspecifiedInvariantsHaveNoSpec proves the honesty case: Go checkers the
// formal spec never formalised (the whole BETA truth) are "unspecified".
func TestUnspecifiedInvariantsHaveNoSpec(t *testing.T) {
	t.Parallel()
	m := mustExtract(t)
	sawBeta := false
	for _, inv := range m.Invariants {
		if inv.Truth == "BETA" {
			sawBeta = true
			if inv.SpecStatus != SpecUnspecified {
				t.Errorf("%s specStatus = %q, want %q (BETA is not in any TLA+ spec)", inv.ID, inv.SpecStatus, SpecUnspecified)
			}
			if inv.Spec != nil {
				t.Errorf("%s should have no spec ref, got %+v", inv.ID, inv.Spec)
			}
		}
	}
	if !sawBeta {
		t.Fatal("no BETA invariants found")
	}
}

// TestValidatesEdgesReferenceRealSpecNodes mirrors TestEdgesReferenceRealNodes
// for the new edge kind: every validates edge links a real invariant to a real
// spec node.
func TestValidatesEdgesReferenceRealSpecNodes(t *testing.T) {
	t.Parallel()
	m := mustExtract(t)
	specNodes := map[string]bool{}
	for _, s := range m.Specs {
		specNodes["spec:"+s.Name] = true
	}
	invNodes := map[string]bool{}
	for _, inv := range m.Invariants {
		invNodes["inv:"+inv.ID] = true
	}
	count := 0
	for _, e := range m.Edges {
		if e.Kind != EdgeValidates {
			continue
		}
		count++
		if !invNodes[e.From] {
			t.Errorf("validates edge from unknown invariant %q", e.From)
		}
		if !specNodes[e.To] {
			t.Errorf("validates edge to unknown spec %q", e.To)
		}
	}
	if count == 0 {
		t.Error("expected at least one validates edge")
	}
}
