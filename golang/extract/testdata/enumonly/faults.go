// Package enumonly is a fixture for the enum-only fault path: a fault enum with
// doc comments but NO catalogue slice and NO name map. It lives outside
// testdata/sample on purpose, so the recursive-discovery tests that scan sample
// are unaffected. It exercises the case the sim fixture cannot (the sim always
// supplies faultNames), where the enum constant is itself the fault id. Never
// compiled — testdata is parsed only.
package enumonly

// Failure enumerates failure modes with no catalogue and no name map.
type Failure int

const (
	// FailTimeout models a request that never returns.
	FailTimeout Failure = iota
	// FailCorrupt models a corrupted write reaching durable storage.
	FailCorrupt
)
