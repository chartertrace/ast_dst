package extract

import (
	"reflect"
	"testing"
)

// TestPresetMatchesDefault guards against the documented presets/sim.json
// drifting from the embedded DefaultConfig — the two must describe the same
// extraction, so "no --config" and "--config presets/sim.json" agree.
func TestPresetMatchesDefault(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig("../presets/sim.json")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := DefaultConfig()
	want.Root = cfg.Root // preset carries a root; the embedded default does not
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("preset != DefaultConfig\n preset = %+v\n want   = %+v", cfg, want)
	}
}

// TestRecursiveDiscoveryMatchesExplicitPackages proves the tool needs no
// package list: discovering the whole tree yields the same model as naming the
// three packages explicitly.
func TestRecursiveDiscoveryMatchesExplicitPackages(t *testing.T) {
	t.Parallel()
	explicit := mustExtract(t)

	cfg := fixtureConfig()
	cfg.Packages = nil // force directory discovery
	recursive, err := Extract(cfg)
	if err != nil {
		t.Fatalf("Extract (recursive): %v", err)
	}

	if recursive.Stats != explicit.Stats {
		t.Errorf("recursive stats %+v != explicit %+v", recursive.Stats, explicit.Stats)
	}
}

// TestTraceCallDrivesFaultEdges proves the operation->fault edge comes from the
// configured trace call: name a call that handler bodies never make and those
// edges disappear.
func TestTraceCallDrivesFaultEdges(t *testing.T) {
	t.Parallel()
	cfg := fixtureConfig()
	cfg.Operations.TraceCall = "noSuchInstrumentationCall"
	m, err := Extract(cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	for _, e := range m.Edges {
		if e.Kind == EdgeOpInjectsFault {
			t.Fatalf("expected no op-injects-fault edges, got %+v", e)
		}
	}
}

func TestReadFieldPositionalAndKeyed(t *testing.T) {
	t.Parallel()
	if got := expandFuncPattern("Check{Title}{Num}", "PAYMENT-11"); got != "CheckPayment11" {
		t.Errorf("expandFuncPattern = %q, want CheckPayment11", got)
	}
	if got := expandFuncPattern("verify_{prefix}_{Num}", "ECONOMICS-7"); got != "verify_economics_7" {
		t.Errorf("expandFuncPattern = %q, want verify_economics_7", got)
	}
}
