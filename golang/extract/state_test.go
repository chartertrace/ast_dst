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
