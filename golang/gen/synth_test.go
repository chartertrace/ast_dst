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
			{Index: 0, Name: "Tick", Weight: 1, Share: 1, Faults: []string{},
				Writes:  []string{"Clock"},
				Effects: []extract.FieldEffect{{Field: "Clock", Op: "inc"}}},
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
	d, rep, err := Synthesize(sampleModel(), "Test", 0, nil)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	tla := d.TLA

	wantContains := []string{
		"VARIABLES Clock, active, Balances",
		"CONSTANT MaxNat",
		"Clock \\in Nat",
		"active \\in BOOLEAN",
		"Init ==",
		"Clock = 0",
		"active = FALSE",
		"Balances = <<>>",
		"StateBound ==",                    // numeric bound for finiteness
		"Clock' = Clock + 1",               // Tick increments Clock -> value transition
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
	if rep.Transitions != 1 || rep.ValueTransitions != 1 || rep.StubOps != 0 {
		t.Errorf("report transitions/value/stub = %d/%d/%d, want 1/1/0", rep.Transitions, rep.ValueTransitions, rep.StubOps)
	}
	if rep.ActiveInvs != 1 || rep.ReferenceInvs != 1 || rep.StubInvs != 1 {
		t.Errorf("report invs active/ref/stub = %d/%d/%d, want 1/1/1", rep.ActiveInvs, rep.ReferenceInvs, rep.StubInvs)
	}
}

func TestSynthesizeNoStateErrors(t *testing.T) {
	t.Parallel()
	if _, _, err := Synthesize(&extract.Model{}, "X", 0, nil); err == nil {
		t.Error("expected an error when there is no state model")
	}
}

func TestSynthesizeOverrides(t *testing.T) {
	t.Parallel()
	// A valid override replaces Tick's recovered body and fills UNCHANGED for the
	// rest; it counts as an override and a value transition.
	d, rep, err := Synthesize(sampleModel(), "Test", 0, map[string]string{"Tick": "Clock' = 5"})
	if err != nil {
		t.Fatalf("Synthesize with override: %v", err)
	}
	if rep.Overrides != 1 {
		t.Errorf("report Overrides = %d, want 1", rep.Overrides)
	}
	for _, want := range []string{"Clock' = 5", "override (config)", "UNCHANGED << active, Balances >>"} {
		if !strings.Contains(d.TLA, want) {
			t.Errorf("override TLA missing %q\n---\n%s", want, d.TLA)
		}
	}

	cases := []struct {
		name      string
		overrides map[string]string
		wantErr   string
	}{
		{"unknown identifier", map[string]string{"Tick": "Clock' = ghost"}, "outside the module's state vocabulary"},
		{"vacuous (primes nothing)", map[string]string{"Tick": "Clock >= 0"}, "primes no declared state variable"},
		{"no such operation", map[string]string{"Nope": "Clock' = 1"}, "matches no operation"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := Synthesize(sampleModel(), "Test", 0, c.overrides)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("got err %v, want it to contain %q", err, c.wantErr)
			}
		})
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
		// Quantifier-bound vars (x) are recognised as local binders, so a predicate
		// over a declared var (balance) now resolves and can be activated.
		{`\A x \in DOMAIN balance: balance[x] >= 0`, true},
		{`\E i \in 1..Clock: i > 0`, true},
		// A foreign identifier in the set or body still fails, even quantified.
		{`\A a \in Accounts: balance[a] >= 0`, false},       // Accounts is foreign
		{`\A x \in DOMAIN balance: foreign[x] >= 0`, false}, // foreign in the body
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
		{"int64", "0", "Nat", kindNum},
		{"uint", "0", "Nat", kindNum},
		{"float64", "0", "Nat", kindNum},
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
	// Tick increments Clock (0→1→2 under StateBound), so the search is multi-state
	// and ClockNonNeg is checked against a real value transition — behavioral.
	if res.TLC.States <= 1 {
		t.Errorf("expected a multi-state search from the Clock transition, got %d states", res.TLC.States)
	}
}

// TestGenerateNilToolchainSkipsVerification locks in the graceful-degradation
// contract: with no toolchain, synthesis still runs and the draft is returned,
// but verification is skipped (SANY/TLC nil, Verified false). The CLI relies on
// this to write an honestly-unverified spec instead of failing when a JVM +
// tla2tools.jar are absent.
func TestGenerateNilToolchainSkipsVerification(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := Generate(ctx, nil, sampleModel(), Options{})
	if err != nil {
		t.Fatalf("Generate with nil toolchain should not error: %v", err)
	}
	if res.Draft == nil || res.Draft.TLA == "" {
		t.Fatal("expected a synthesised draft even without a toolchain")
	}
	if res.Verified {
		t.Error("Verified must be false when verification was skipped")
	}
	if res.SANY != nil || res.TLC != nil {
		t.Errorf("no tool should run without a toolchain: SANY=%v TLC=%v", res.SANY, res.TLC)
	}
}

func TestOpActionValueForms(t *testing.T) {
	t.Parallel()
	num := synthVar{tlaName: "Clock", field: "Clock", kind: kindNum}
	b := synthVar{tlaName: "flag", field: "flag", kind: kindBool}
	str := synthVar{tlaName: "status", field: "status", kind: kindOther, typeOK: "STRING"}
	cases := []struct {
		eff    extract.FieldEffect
		v      synthVar
		clause string
		value  bool
	}{
		{extract.FieldEffect{Op: "inc"}, num, "Clock' = Clock + 1", true},
		{extract.FieldEffect{Op: "add", Value: "5"}, num, "Clock' = Clock + 5", true},
		{extract.FieldEffect{Op: "setNum", Value: "3"}, num, "Clock' = 3", true},
		{extract.FieldEffect{Op: ""}, num, "Clock' \\in 0..MaxNat", false}, // no form -> bound
		{extract.FieldEffect{Op: "setBool", Value: "TRUE"}, b, "flag' = TRUE", true},
		{extract.FieldEffect{Op: ""}, b, "flag' \\in BOOLEAN", false},
		{extract.FieldEffect{Op: "setStr", Value: `"done"`}, str, `status' = "done"`, true},
	}
	for _, c := range cases {
		clause, value, modelled := fieldClause(c.v, c.eff)
		if !modelled || clause != c.clause || value != c.value {
			t.Errorf("fieldClause(%s, %+v) = (%q,%v,%v), want (%q,%v,true)", c.v.tlaName, c.eff, clause, value, modelled, c.clause, c.value)
		}
	}
}
