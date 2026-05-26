package extract

import "testing"

// TestCatalogueOnlyFaults covers the catalogue-only fault path: a positional
// `[]struct{id,label}` slice with no enum type and no name map. The sim fixture
// always pairs a catalogue with an enum, so this is the only check that the
// slice alone yields id + label + order.
func TestCatalogueOnlyFaults(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Root: "testdata/cataloguefaults",
		Faults: FaultConfig{
			StructSliceVar: "Faults",
			IDField:        "0",
			LabelField:     "1",
		},
	}
	m, err := Extract(cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(m.Faults) != 2 {
		t.Fatalf("faults = %d, want 2", len(m.Faults))
	}
	// Order and both positional fields must survive.
	if m.Faults[0].ID != "disk_full" || m.Faults[1].ID != "net_partition" {
		t.Errorf("ids/order wrong: %q, %q", m.Faults[0].ID, m.Faults[1].ID)
	}
	if m.Faults[0].Label != "Disk reached capacity" {
		t.Errorf("label not read from catalogue: %q", m.Faults[0].Label)
	}
	// No enum source, so Enum stays empty — but the catalogue still produces faults.
	if m.Faults[0].Enum != "" {
		t.Errorf("expected empty Enum with no enum source, got %q", m.Faults[0].Enum)
	}
}

// TestInterfaceStateExtraction covers state extraction when the state type is an
// interface (Kind=="interface"), surfacing its methods as variables — the
// interfaceVars path the struct-based sim fixture never reaches.
func TestInterfaceStateExtraction(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Root:  "testdata/ifacestate",
		State: StateConfig{TypeName: "Store"},
	}
	m, err := Extract(cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if m.State == nil {
		t.Fatal("state is nil; expected the interface to be extracted")
	}
	if m.State.Kind != "interface" {
		t.Errorf("kind = %q, want interface", m.State.Kind)
	}
	if len(m.State.Variables) != 3 {
		t.Fatalf("variables = %d, want 3 (Put, Delete, Get)", len(m.State.Variables))
	}
	var put *StateVar
	for i := range m.State.Variables {
		if m.State.Variables[i].Name == "Put" {
			put = &m.State.Variables[i]
		}
	}
	if put == nil {
		t.Fatal("Put method not surfaced as a state variable")
	}
	if put.Type == "" {
		t.Error("Put has no rendered signature")
	}
	if put.Doc == "" {
		t.Error("Put lost its doc comment")
	}
}
