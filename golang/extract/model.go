// Package extract walks an arbitrary "sim-shaped" Go codebase with go/ast and
// produces a structured, JSON-serialisable model of a deterministic simulation
// test (DST): its faults, invariants (grouped by Truth), and the weighted
// operations that drive the state machine — plus the edges between them that
// the source actually encodes, and optionally the formal TLA+ specification
// each invariant validates.
//
// It is config-driven (see Config): every codebase-specific name — the fault
// catalogue, the invariant catalogue, the op weight table, the instrumentation
// call linking ops to faults — is supplied by the caller, not hard-coded.
// DefaultConfig returns the names CharterTrace's primary-server/sim happens to
// use, which serves as a worked example.
//
// Nothing in the output is hand-curated: every node and edge is derived from
// the target's own source, so adding a fault or changing a weight is reflected
// simply by regenerating the model.
package extract

// Model is the complete extracted picture of the DST, ready to serialise to
// JSON for the viewer.
type Model struct {
	GeneratedAt string      `json:"generatedAt"`
	Source      Source      `json:"source"`
	Truths      []Truth     `json:"truths"`
	Invariants  []Invariant `json:"invariants"`
	Faults      []Fault     `json:"faults"`
	Operations  []Operation `json:"operations"`
	// State is the mutable state model (TLA+ VARIABLES candidates) the operations
	// act on. Populated only when a state type is configured (see StateConfig);
	// the structural viewer ignores it, the TLA+ generator needs it. Nil otherwise.
	State *StateModel `json:"state,omitempty"`
	// Specs is the formal TLA+ invariant catalogue the sim's checkers validate.
	// Empty unless a TLA+ spec dir is configured (see TLAConfig).
	Specs []SpecInvariant `json:"specs,omitempty"`
	Edges []Edge          `json:"edges"`
	Stats Stats           `json:"stats"`
	// Meta carries the display labels for an alternate extraction mode. Nil for
	// the default DST model (the viewer then uses its built-in DST labels); set
	// by the generic `structure` mode so the same viewer reads "Functions /
	// Types / Packages" instead of "Operations / Faults / Invariants".
	Meta *Meta `json:"meta,omitempty"`
}

// Meta tells the viewer how to label a model whose buckets don't carry their DST
// meaning. The node/edge shape is identical; only the words change.
type Meta struct {
	Mode   string `json:"mode"`  // "structure" (DST models omit Meta entirely)
	Title  string `json:"title"` // header title, e.g. "Go structure"
	Labels Labels `json:"labels"`
}

// Labels names the four node buckets for the current mode.
type Labels struct {
	Operations string `json:"operations"` // column 1
	Faults     string `json:"faults"`     // column 2
	Invariants string `json:"invariants"` // column 3 items
	Truths     string `json:"truths"`     // column 3 groups
}

// Source records where the model came from, so a stale artifact is obvious.
type Source struct {
	SimPath    string `json:"simPath"`
	FilesRead  int    `json:"filesRead"`
	ToolModule string `json:"toolModule"`
}

// Loc is a source location, written relative to the sim root so the viewer can
// link a node back to the line that defines it.
type Loc struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// Truth is one of the nine categories the invariants partition into (SPATIAL,
// PAYMENT, ECONOMICS, …). Count is how many invariants carry that prefix.
type Truth struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Invariant is one TLA+-derived safety property the sim checks after every op.
type Invariant struct {
	ID    string `json:"id"`    // truth-prefixed, e.g. "PAYMENT-11"
	Label string `json:"label"` // CamelCase name, e.g. "DualStateConsistency"
	Truth string `json:"truth"` // ID prefix, e.g. "PAYMENT"
	Doc   string `json:"doc,omitempty"`
	Loc   *Loc   `json:"loc,omitempty"`
	// Spec links this runtime checker to the formal TLA+ invariant it validates,
	// when one was found. SpecStatus classifies the coverage (see the constants).
	Spec       *SpecRef `json:"spec,omitempty"`
	SpecStatus string   `json:"specStatus,omitempty"`
}

// Invariant spec-coverage states, set on Invariant.SpecStatus.
const (
	// SpecValidated: a TLA+ invariant in the model-checked .cfg set has a live
	// Go checker.
	SpecValidated = "validated"
	// SpecUnchecked: a TLA+ invariant exists but the .cfg never model-checks it.
	SpecUnchecked = "unchecked-spec"
	// SpecUnspecified: a Go checker with no formal TLA+ counterpart (e.g. the
	// ECONOMICS truth the spec never formalised).
	SpecUnspecified = "unspecified"
)

// SpecInvariant is one invariant declared in a TLA+ specification. Predicate is
// the verbatim operator body; Checked reports whether the TLC .cfg model-checks
// it. Like every node, it carries the source location it was parsed from.
type SpecInvariant struct {
	Name      string `json:"name"`      // TLA+ operator name, e.g. "NoNegativeBalances"
	Predicate string `json:"predicate"` // verbatim definition body
	SpecFile  string `json:"specFile"`  // file the definition lives in, e.g. "PayoutFlowV3.tla"
	Checked   bool   `json:"checked"`   // named in the TLC .cfg INVARIANT set
	Doc       string `json:"doc,omitempty"`
	Loc       *Loc   `json:"loc,omitempty"`
	// Provenance: set when this spec was machine-generated (see package gen) rather
	// than hand-written. Generated reports authorship; Verified reports that TLC
	// model-checked the spec clean; TLCStates/TLCDepth are the search it covered.
	// Behavioral distinguishes a meaningful check (an active invariant verified
	// against ≥1 real value transition) from a merely well-formed one (TLC passed,
	// but the transitions carry no value semantics, so the ✓ proves little).
	Generated  bool `json:"generated,omitempty"`
	Verified   bool `json:"verified,omitempty"`
	Behavioral bool `json:"behavioral,omitempty"`
	TLCStates  int  `json:"tlcStates,omitempty"`
	TLCDepth   int  `json:"tlcDepth,omitempty"`
	TLCMaxNat  int  `json:"tlcMaxNat,omitempty"` // numeric bound (0..N) the TLC search used
	// Counterexample is the trace TLC produced when this generated invariant was
	// violated, denormalised here so the viewer can render the failing path.
	Counterexample []TraceState `json:"counterexample,omitempty"`
}

// TraceState is one state in a TLC counterexample path: step number, the action
// that produced it, and each variable's rendered value. Mirrors gen.TraceState
// (kept here to avoid an extract→gen import cycle).
type TraceState struct {
	Num    int               `json:"num"`
	Action string            `json:"action,omitempty"`
	Vars   map[string]string `json:"vars"`
}

// StateModel is the mutable state the operations act on, the basis for the TLA+
// VARIABLES. Kind is "struct" (Variables are its fields) or "interface"
// (Variables are its mutating methods); the TLA+ generator abstracts each.
type StateModel struct {
	TypeName  string     `json:"typeName"` // e.g. "Server" or "StateServer"
	Kind      string     `json:"kind"`     // "struct" | "interface"
	Variables []StateVar `json:"variables"`
	Loc       *Loc       `json:"loc,omitempty"`
}

// StateVar is one component of the state: a struct field or an interface method.
// Type is the rendered Go type (a field's type, or a method's signature).
type StateVar struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Doc  string `json:"doc,omitempty"`
	Loc  *Loc   `json:"loc,omitempty"`
}

// SpecRef is the back-reference from a Go invariant to the spec invariant it
// validates, denormalised so the viewer needn't cross-index Specs.
type SpecRef struct {
	Name     string `json:"name"`
	SpecFile string `json:"specFile"`
	Checked  bool   `json:"checked"`
	Loc      *Loc   `json:"loc,omitempty"`
	// Provenance mirrored from the SpecInvariant so the viewer can badge an
	// invariant as machine-generated / TLC-verified without cross-indexing Specs.
	Generated  bool `json:"generated,omitempty"`
	Verified   bool `json:"verified,omitempty"`
	Behavioral bool `json:"behavioral,omitempty"`
	TLCMaxNat  int  `json:"tlcMaxNat,omitempty"`
}

// Fault is one injectable failure mode. Enum is the Go constant name; ID is the
// snake_case name the engine uses on the wire and in metrics.
type Fault struct {
	ID    string `json:"id"`    // "crash_after_stripe"
	Enum  string `json:"enum"`  // "FaultCrashAfterStripe"
	Label string `json:"label"` // human summary from the catalogue
	Doc   string `json:"doc,omitempty"`
	Loc   *Loc   `json:"loc,omitempty"`
}

// Operation is one weighted action the engine can dispatch. Faults lists the
// fault enums its handler body actually records via e.recordFault(...).
type Operation struct {
	Index   int      `json:"index"`
	Handler string   `json:"handler"` // "opCreateShipment"
	Name    string   `json:"name"`    // "CreateShipment" (op-prefix stripped)
	Weight  float64  `json:"weight"`
	Share   float64  `json:"share"` // weight / total, 0..1
	Note    string   `json:"note,omitempty"`
	Faults  []string `json:"faults"` // fault enum names recorded in the body
	// Writes lists the state-struct field names this handler assigns to. The TLA+
	// generator turns these into the operation's transition; empty when no state
	// field is written (or state is behind an interface). Best-effort, syntactic.
	Writes []string `json:"writes,omitempty"`
	// Effects refines Writes with *how* each field changes when it is recoverable
	// from a simple statement form (++/--, += literal, = literal). The generator
	// uses these to emit value transitions (`f' = f + 1`) instead of a
	// nondeterministic bound; a field with no recoverable form is absent here.
	Effects []FieldEffect `json:"effects,omitempty"`
	Loc     *Loc          `json:"loc,omitempty"`
}

// FieldEffect is the recovered shape of a write to one state field, when it
// matches a simple deterministic form. Op names the form; Value is the rendered
// literal operand for the forms that take one.
type FieldEffect struct {
	Field string `json:"field"`
	// Op ∈ {inc, dec, add, sub, setNum, setBool, setStr}. A field written in
	// several conflicting ways within one handler is omitted (the generator falls
	// back to a nondeterministic bound), so every FieldEffect here is unambiguous.
	Op    string `json:"op"`
	Value string `json:"value,omitempty"` // literal operand: "1", "TRUE", "\"done\""
}

// Edge kinds.
const (
	EdgeTruthHasInvariant = "truth-has-invariant"
	EdgeOpInjectsFault    = "op-injects-fault"
	EdgeMentionsInvariant = "mentions-invariant" // from doc-comment references
	EdgeValidates         = "validates"          // Go invariant -> TLA+ spec invariant
)

// Edge connects two nodes. Node IDs are namespaced: "truth:PAYMENT",
// "inv:PAYMENT-11", "fault:crash_after_stripe", "op:3", "spec:NoNegativeBalances".
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// Stats are convenience totals so the viewer needn't recompute them.
type Stats struct {
	Truths      int     `json:"truths"`
	Invariants  int     `json:"invariants"`
	Faults      int     `json:"faults"`
	Operations  int     `json:"operations"`
	TotalWeight float64 `json:"totalWeight"`
	Edges       int     `json:"edges"`
	// Spec-coverage totals. Zero when no TLA+ spec is configured.
	SpecsTotal    int `json:"specsTotal"`
	Validated     int `json:"validated"`     // invariants with specStatus "validated"
	UncheckedSpec int `json:"uncheckedSpec"` // invariants with specStatus "unchecked-spec"
	Unspecified   int `json:"unspecified"`   // invariants with specStatus "unspecified"
}
