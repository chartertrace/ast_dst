package extract

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config describes where, in an arbitrary Go codebase, the four domain concepts
// live and what they're called. Nothing about CharterTrace's sim is baked into
// the extractor any more — DefaultConfig() merely supplies the sim's names.
// Point the tool at any repo by writing a config (see presets/sim.json) that
// names that repo's catalogue vars, enum type, checker-method pattern, and —
// for "any tracing setup" — the instrumentation call that records a fault.
type Config struct {
	// Root is the codebase directory to parse (overridden by --root).
	Root string `json:"root,omitempty"`
	// Packages lists sub-directories of Root to parse. Empty = recurse into
	// every directory under Root that contains Go files.
	Packages []string `json:"packages,omitempty"`

	Faults     FaultConfig     `json:"faults"`
	Invariants InvariantConfig `json:"invariants"`
	Operations OperationConfig `json:"operations"`
	TLA        TLAConfig       `json:"tla"`
	State      StateConfig     `json:"state"`
}

// StateConfig names the mutable state type the operations act on, so the TLA+
// generator has VARIABLES to abstract. Optional: an empty TypeName leaves
// Model.State nil and the structural pipeline unaffected.
type StateConfig struct {
	// TypeName is a Go struct or interface whose fields/methods are the state,
	// e.g. "Server" or "StateServer".
	TypeName string `json:"typeName,omitempty"`
	// Package optionally narrows the search to one configured package dir.
	Package string `json:"package,omitempty"`
}

// TLAConfig locates the formal TLA+ specification the invariant catalogue is
// derived from, so the extractor can bind each Go invariant to the spec
// invariant it validates at runtime and report coverage. All fields optional;
// an empty SpecDir disables the layer entirely (the model has no specs and
// every invariant's specStatus stays empty), so non-TLA codebases are
// unaffected.
type TLAConfig struct {
	// SpecDir holds the *.tla files, relative to Root (e.g. "../docs/tla").
	SpecDir string `json:"specDir,omitempty"`
	// CfgFile is the TLC config (within SpecDir) whose INVARIANT lines name the
	// model-checked subset, e.g. "PayoutFlow.cfg".
	CfgFile string `json:"cfgFile,omitempty"`
	// MatchByLabel binds a Go invariant to a spec invariant when the Go Label
	// equals the TLA+ operator name (the sim's convention: PAYMENT-1's label is
	// "NoNegativeBalances", matching the TLA+ definition of the same name).
	MatchByLabel bool `json:"matchByLabel,omitempty"`
}

// FaultConfig locates the injectable-fault catalogue.
type FaultConfig struct {
	// StructSliceVar is `var X = []T{ ... }` giving fault id + label, in order.
	StructSliceVar string `json:"structSliceVar,omitempty"`
	IDField        string `json:"idField,omitempty"`    // field ref: "0"/"ID"
	LabelField     string `json:"labelField,omitempty"` // field ref: "1"/"Label"
	// EnumType is a Go const type (e.g. "FaultType") whose const block supplies
	// enum names + doc comments. Optional.
	EnumType string   `json:"enumType,omitempty"`
	EnumSkip []string `json:"enumSkip,omitempty"` // const names to ignore (e.g. sentinels)
	// NameMapVar is `var X = map[EnumType]string{ Enum: "id" }`, linking the Go
	// enum constant to the wire id used in the catalogue. Optional.
	NameMapVar string `json:"nameMapVar,omitempty"`
}

// InvariantConfig locates the safety-property catalogue.
type InvariantConfig struct {
	StructSliceVar    string `json:"structSliceVar,omitempty"`
	IDField           string `json:"idField,omitempty"`
	LabelField        string `json:"labelField,omitempty"`
	GroupFromIDPrefix bool   `json:"groupFromIDPrefix,omitempty"` // "PAYMENT-1" -> truth "PAYMENT"
	// DocFuncPattern is a template naming the checker method that documents an
	// invariant, expanded per id. Tokens: {ID} {Prefix} {prefix} {Title} {Num}.
	// e.g. "Check{Title}{Num}" maps "PAYMENT-11" -> "CheckPayment11". Optional.
	DocFuncPattern string `json:"docFuncPattern,omitempty"`
}

// OperationConfig locates the weighted operation table and the instrumentation
// call that links an operation to the faults it injects.
type OperationConfig struct {
	// TableVar is `var X = [N]func(...){ handlerA, handlerB, ... }` in order.
	TableVar string `json:"tableVar,omitempty"`
	// WeightsFunc is the function that assigns the weight literal; WeightsVar is
	// the assigned (field or var) name holding `[N]float64{...}`.
	WeightsFunc string `json:"weightsFunc,omitempty"`
	WeightsVar  string `json:"weightsVar,omitempty"`
	// NamePrefix is stripped from a handler name to derive the display Name.
	NamePrefix string `json:"namePrefix,omitempty"`
	// HandlerFuncPrefix selects which funcs to scan for trace calls. Defaults to
	// NamePrefix when empty.
	HandlerFuncPrefix string `json:"handlerFuncPrefix,omitempty"`
	// TraceCall is the function/method whose call sites name an injected fault —
	// "recordFault", "span.AddEvent", "tracer.Record", etc. TraceArgIndex picks
	// which argument carries the fault identity.
	TraceCall     string `json:"traceCall,omitempty"`
	TraceArgIndex int    `json:"traceArgIndex,omitempty"`
}

// DefaultConfig returns the CharterTrace sim preset — the names the original
// hard-coded extractor used, so running with no --config reproduces it exactly.
func DefaultConfig() Config {
	return Config{
		Packages: []string{"catalog", "engine", "state"},
		Faults: FaultConfig{
			StructSliceVar: "Faults",
			IDField:        "0",
			LabelField:     "1",
			EnumType:       "FaultType",
			EnumSkip:       []string{"FaultNone", "NumFaultTypes"},
			NameMapVar:     "faultNames",
		},
		Invariants: InvariantConfig{
			StructSliceVar:    "Invariants",
			IDField:           "ID",
			LabelField:        "Label",
			GroupFromIDPrefix: true,
			DocFuncPattern:    "Check{Title}{Num}",
		},
		Operations: OperationConfig{
			TableVar:      "opTable",
			WeightsFunc:   "initWeights",
			WeightsVar:    "opWeights",
			NamePrefix:    "op",
			TraceCall:     "recordFault",
			TraceArgIndex: 0,
		},
		TLA: TLAConfig{
			SpecDir:      "../docs/tla",
			CfgFile:      "PayoutFlow.cfg",
			MatchByLabel: true,
		},
		State: StateConfig{
			TypeName: "StateServer",
			Package:  "state",
		},
	}
}

// LoadConfig reads a JSON config file. Fields left unset keep DefaultConfig's
// value, so a config can override only what differs.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	// Decode onto the default so omitted fields fall back to it.
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

// handlerPrefix returns the prefix used to select operation handler functions.
func (o OperationConfig) handlerPrefix() string {
	if o.HandlerFuncPrefix != "" {
		return o.HandlerFuncPrefix
	}
	return o.NamePrefix
}
