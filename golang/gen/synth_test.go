package gen

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chartertrace/ast_dst/golang/extract"
)

// sampleModel is a small in-memory model exercising every synthesis path: a
// typed scalar, a bool, an opaque map; an op; an invariant whose predicate
// resolves to a declared var (active), one whose predicate is foreign
// (reference), and one with no predicate (stub).
func sampleModel() *extract.Model {
	return &extract.Model{
		Source: extract.Source{SimPath: "test"},
		State: &extract.StateModel{
			TypeName: "World", Kind: "struct",
			Variables: []extract.StateVar{
				{Name: "Clock", Type: "int64"},
				{Name: "active", Type: "bool"},
				{Name: "Balances", Type: "map[string]int64"},
			},
		},
		Operations: []extract.Operation{
			{Index: 0, Name: "Tick", Weight: 1, Share: 1, Faults: []string{}, Writes: []string{"Clock"}},
		},
		Invariants: []extract.Invariant{
			{ID: "A-1", Label: "ClockNonNeg", Spec: &extract.SpecRef{Name: "ClockNonNeg"}, SpecStatus: "validated"},
			{ID: "A-2", Label: "BalancesNonNeg", Spec: &extract.SpecRef{Name: "BalancesNonNeg"}, SpecStatus: "unchecked-spec"},
			{ID: "A-3", Label: "NoPredicate", SpecStatus: "unspecified"},
		},
		Specs: []extract.SpecInvariant{
			{Name: "ClockNonNeg", Predicate: "Clock >= 0"},
			{Name: "BalancesNonNeg", Predicate: `\A a \in Accounts: balance[a] >= 0`}, // foreign vocab
		},
	}
}

func TestSynthesizeStructure(t *testing.T) {
	t.Parallel()
	d, rep, err := Synthesize(sampleModel(), "Test")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	tla := d.TLA

	wantContains := []string{
		"VARIABLES Clock, active, Balances",
		"CONSTANT MaxNat",
		"Clock \\in 0..MaxNat",
		"active \\in BOOLEAN",
		"Init ==",
		"Clock = 0",
		"active = FALSE",
		"Balances = <<>>",
		"Clock' \\in 0..MaxNat",            // Tick writes Clock -> real transition
		"UNCHANGED << active, Balances >>", // the rest held
		"Spec == Init /\\ [][Next]_vars",
		"ClockNonNeg ==",      // active invariant
		"/\\ ClockNonNeg",     // listed in AllInvariants
		"reference predicate", // foreign one carried as comment
	}
	for _, s := range wantContains {
		if !strings.Contains(tla, s) {
			t.Errorf("generated TLA missing %q\n---\n%s", s, tla)
		}
	}
	// The foreign predicate must NOT be activated.
	if strings.Contains(tla, "BalancesNonNeg ==") {
		t.Error("foreign-vocabulary predicate should be a reference comment, not active")
	}
	if rep.Variables != 3 || rep.Typed != 2 {
		t.Errorf("report vars/typed = %d/%d, want 3/2", rep.Variables, rep.Typed)
	}
	if rep.Transitions != 1 || rep.StubOps != 0 {
		t.Errorf("report transitions/stubOps = %d/%d, want 1/0", rep.Transitions, rep.StubOps)
	}
	if rep.ActiveInvs != 1 || rep.ReferenceInvs != 1 || rep.StubInvs != 1 {
		t.Errorf("report invs active/ref/stub = %d/%d/%d, want 1/1/1", rep.ActiveInvs, rep.ReferenceInvs, rep.StubInvs)
	}
}

func TestSynthesizeNoStateErrors(t *testing.T) {
	t.Parallel()
	if _, _, err := Synthesize(&extract.Model{}, "X"); err == nil {
		t.Error("expected an error when there is no state model")
	}
}

func TestPredicateResolves(t *testing.T) {
	t.Parallel()
	vars := map[string]bool{"Clock": true, "balance": true}
	cases := []struct {
		pred string
		want bool
	}{
		{"Clock >= 0", true},
		{"balance = Clock", true},
		// Quantified predicates introduce bound vars (x, a) we can't cheaply tell
		// from foreign free vars, so they're conservatively treated as reference.
		{`\A x \in DOMAIN balance: balance[x] >= 0`, false},
		{`\A a \in Accounts: balance[a] >= 0`, false}, // Accounts is foreign too
		{"TRUE", false}, // no variable referenced
	}
	for _, c := range cases {
		if got := predicateResolves(c.pred, vars); got != c.want {
			t.Errorf("predicateResolves(%q) = %v, want %v", c.pred, got, c.want)
		}
	}
}

func TestTlaTypeOf(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, init, typeOK, kind string }{
		{"int64", "0", "0..MaxNat", kindNum},
		{"uint", "0", "0..MaxNat", kindNum},
		{"float64", "0", "0..MaxNat", kindNum},
		{"bool", "FALSE", "BOOLEAN", kindBool},
		{"string", `""`, "STRING", kindOther},
		{"map[string]int", "<<>>", "", kindOther},
		{"[]string", "<<>>", "", kindOther},
		{"*Foo", "<<>>", "", kindOther},
	}
	for _, c := range cases {
		init, typeOK, kind := tlaTypeOf(c.in)
		if init != c.init || typeOK != c.typeOK || kind != c.kind {
			t.Errorf("tlaTypeOf(%q) = (%q,%q,%q), want (%q,%q,%q)", c.in, init, typeOK, kind, c.init, c.typeOK, c.kind)
		}
	}
}

// TestGenerateVerifiesSynthesized drives the real toolchain: the synthesised
// skeleton must SANY-parse and TLC-verify (initial state satisfies TypeOK and the
// active ClockNonNeg invariant).
func TestGenerateVerifiesSynthesized(t *testing.T) {
	tc := testToolchain(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	res, err := Generate(ctx, tc, sampleModel(), Options{Workers: 2})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.SANY == nil || !res.SANY.OK {
		t.Fatalf("synthesised spec should parse, got %+v", res.SANY)
	}
	if !res.Verified {
		t.Fatalf("synthesised spec should verify, got TLC %+v", res.TLC)
	}
	// Tick writes Clock (bounded 0..MaxNat), so the state space is more than the
	// initial state — a real multi-state model-check, not just Init.
	if res.TLC.States <= 1 {
		t.Errorf("expected a multi-state search from the Clock transition, got %d states", res.TLC.States)
	}
}
