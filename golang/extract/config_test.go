package extract

import (
	"reflect"
	"strings"
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

// TestValidateAcceptsGoodConfigs proves the validator passes the real configs
// the tests and presets rely on, so it can't silently start rejecting valid use.
func TestValidateAcceptsGoodConfigs(t *testing.T) {
	t.Parallel()
	if err := fixtureConfig().Validate(); err != nil {
		t.Errorf("fixtureConfig should validate: %v", err)
	}
	// Recurse mode (no explicit packages) over an existing root.
	rec := fixtureConfig()
	rec.Packages = nil
	if err := rec.Validate(); err != nil {
		t.Errorf("recurse-mode config should validate: %v", err)
	}
}

// TestValidateCatchesMisconfig covers each static check, since these are exactly
// the cases that previously produced a silent empty section instead of an error.
func TestValidateCatchesMisconfig(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*Config)
		want   string // substring expected in the error
	}{
		{"missing root", func(c *Config) { c.Root = "testdata/does-not-exist" }, "root"},
		{"missing package", func(c *Config) { c.Packages = []string{"catalog", "nope"} }, `package "nope"`},
		{"negative trace index", func(c *Config) { c.Operations.TraceArgIndex = -1 }, "traceArgIndex"},
		{"cfg without specDir", func(c *Config) { c.TLA.SpecDir = ""; c.TLA.CfgFile = "X.cfg" }, "specDir"},
		{"half-set weights", func(c *Config) { c.Operations.WeightsVar = "" }, "weights"},
		{"slice without fields", func(c *Config) { c.Faults.IDField = "" }, "idField/labelField"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := fixtureConfig()
			tc.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected an error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should mention %q", err.Error(), tc.want)
			}
		})
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
