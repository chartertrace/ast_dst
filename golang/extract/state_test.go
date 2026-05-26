package extract

import (
	"slices"
	"testing"
)

// stateConfig extends the fixture config to name the fixture's state struct.
func stateConfig() Config {
	cfg := fixtureConfig()
	cfg.State = StateConfig{TypeName: "World", Package: "engine"}
	return cfg
}

func TestExtractStateStruct(t *testing.T) {
	t.Parallel()
	m, err := Extract(stateConfig())
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if m.State == nil {
		t.Fatal("expected a state model, got nil")
	}
	if m.State.Kind != "struct" {
		t.Errorf("Kind = %q, want struct", m.State.Kind)
	}
	if m.State.Loc == nil {
		t.Error("state model has no source location")
	}

	// All four fields (including the unexported one) should surface, with types.
	want := map[string]string{
		"Clock":     "int64",
		"Shipments": "map[string]string",
		"Balances":  "map[string]int64",
		"pending":   "[]string",
	}
	got := map[string]string{}
	for _, v := range m.State.Variables {
		got[v.Name] = v.Type
	}
	if len(got) != len(want) {
		t.Fatalf("got %d state vars %v, want %d", len(got), got, len(want))
	}
	for name, typ := range want {
		if got[name] != typ {
			t.Errorf("var %s type = %q, want %q", name, got[name], typ)
		}
	}

	// Doc comments should be carried through for documented fields.
	for _, v := range m.State.Variables {
		if v.Name == "Clock" && v.Doc == "" {
			t.Error("Clock field lost its doc comment")
		}
	}
}

func TestExtractOperationWriteSets(t *testing.T) {
	t.Parallel()
	m, err := Extract(stateConfig())
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	// The fixture handlers mutate World fields: Tick→Clock, Create/Complete→
	// Shipments, Transfer→Balances. Effect analysis should recover those.
	want := map[string][]string{
		"CreateShipment":   {"Shipments"},
		"CompleteShipment": {"Shipments"},
		"Transfer":         {"Balances"},
		"Tick":             {"Clock"},
	}
	for _, op := range m.Operations {
		w, ok := want[op.Name]
		if !ok {
			continue
		}
		if !slices.Equal(op.Writes, w) {
			t.Errorf("op %s writes = %v, want %v", op.Name, op.Writes, w)
		}
	}

	// Tick is `w.Clock++` — a recoverable value form; the map writes are indexed,
	// so their form is not recovered (Op stays empty, generator falls back).
	for _, op := range m.Operations {
		switch op.Name {
		case "Tick":
			if len(op.Effects) != 1 || op.Effects[0].Field != "Clock" || op.Effects[0].Op != "inc" {
				t.Errorf("Tick effects = %+v, want one {Clock inc}", op.Effects)
			}
		case "Transfer": // w.Balances["d"] += 1 — indexed write, form not recovered
			if len(op.Effects) != 1 || op.Effects[0].Field != "Balances" || op.Effects[0].Op != "" {
				t.Errorf("Transfer effects = %+v, want one {Balances <none>}", op.Effects)
			}
		}
	}
}

func TestExtractStateAbsentTypeIsNil(t *testing.T) {
	t.Parallel()
	cfg := fixtureConfig()
	cfg.State = StateConfig{TypeName: "NoSuchType"}
	m, err := Extract(cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if m.State != nil {
		t.Errorf("expected nil state for absent type, got %+v", m.State)
	}
}

func TestExtractStateDisabledByDefault(t *testing.T) {
	t.Parallel()
	// fixtureConfig inherits DefaultConfig's State (TypeName "StateServer"), which
	// the fixture lacks, so State stays nil — the structural pipeline is unaffected.
	m := mustExtract(t)
	if m.State != nil {
		t.Errorf("fixture has no StateServer; expected nil state, got %+v", m.State)
	}
}

// TestExtractStateFlattensEmbeds covers the composition-built state case: a value
// embed and a pointer embed are flattened into promoted fields (with Via set), an
// external embed it can't resolve is kept opaque (with Unresolved set), and a
// direct field shadows a same-named promoted one.
func TestExtractStateFlattensEmbeds(t *testing.T) {
	t.Parallel()
	cfg := Config{Root: "testdata/embed", State: StateConfig{TypeName: "World"}}
	m, err := Extract(cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if m.State == nil || m.State.Kind != "struct" {
		t.Fatalf("expected a struct state model, got %+v", m.State)
	}

	vars := map[string]StateVar{}
	for _, v := range m.State.Variables {
		if _, dup := vars[v.Name]; dup {
			t.Errorf("var %q appears twice — shadowing should keep only the shallower one", v.Name)
		}
		vars[v.Name] = v
	}

	// Direct fields: present, no Via.
	for _, name := range []string{"Clock", "Label"} {
		v, ok := vars[name]
		if !ok {
			t.Errorf("missing direct field %q", name)
		} else if v.Via != "" || v.Unresolved != "" {
			t.Errorf("direct field %q has Via=%q Unresolved=%q, want both empty", name, v.Via, v.Unresolved)
		}
	}
	// Label is the direct string field, not the promoted Meta.Label (shadowing).
	if v := vars["Label"]; v.Type != "string" {
		t.Errorf("Label type = %q, want string (direct field should shadow promoted Meta.Label)", v.Type)
	}

	// Promoted fields: present, with the embed path in Via.
	wantVia := map[string]string{"Version": "Meta", "Count": "Audit"}
	for name, via := range wantVia {
		v, ok := vars[name]
		if !ok {
			t.Errorf("missing promoted field %q (embed not flattened)", name)
		} else if v.Via != via {
			t.Errorf("promoted field %q Via = %q, want %q", name, v.Via, via)
		} else if v.Unresolved != "" {
			t.Errorf("promoted field %q should be resolved, got Unresolved=%q", name, v.Unresolved)
		}
	}

	// External embed (sync.Mutex): kept opaque with an honest reason, never dropped.
	mu, ok := vars["Mutex"]
	if !ok {
		t.Fatal("external embed sync.Mutex was dropped; expected it kept with Unresolved set")
	}
	if mu.Unresolved == "" {
		t.Errorf("Mutex.Unresolved is empty; want a reason it couldn't be flattened")
	}
}
