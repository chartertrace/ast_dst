// Package catalog is part of a synthetic fixture codebase used by the extract
// tests. It is never compiled by the go tool (testdata directories are ignored);
// it exists only to be parsed by the extractor, exactly as a real target
// codebase would be. It deliberately uses the same catalogue/enum names as the
// built-in preset (Faults, FaultType, faultNames, Invariants) so the tests can
// run DefaultConfig() against it unchanged.
package catalog

// FaultType enumerates the injectable failure modes. FaultNone and
// NumFaultTypes are sentinels the preset's enumSkip drops.
type FaultType int

const (
	FaultNone FaultType = iota
	// FaultTransferFail simulates a failed external transfer.
	FaultTransferFail
	// FaultCreditRace races two concurrent credits against one balance. Violates ALPHA-1.
	FaultCreditRace
	// FaultCrashAfterWrite crashes the process after a write has committed.
	FaultCrashAfterWrite
	// FaultClockSkew advances one node's clock out of step with the others.
	FaultClockSkew
	NumFaultTypes
)

// FaultInfo is one row of the positional fault catalogue: {id, label}.
type FaultInfo struct {
	id    string
	label string
}

// Faults is the fault catalogue in wire order; read positionally (0=id, 1=label).
var Faults = []FaultInfo{
	{"transfer_fail", "External transfer failure"},
	{"credit_race", "Concurrent credit race"},
	{"crash_after_write", "Crash after committed write"},
	{"clock_skew", "Node clock skew"},
}

// faultNames links each Go enum constant to the wire id used in the catalogue.
var faultNames = map[FaultType]string{
	FaultTransferFail:    "transfer_fail",
	FaultCreditRace:      "credit_race",
	FaultCrashAfterWrite: "crash_after_write",
	FaultClockSkew:       "clock_skew",
}
