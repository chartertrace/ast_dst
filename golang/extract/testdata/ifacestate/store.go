// Package ifacestate is a fixture for interface-kind state extraction: the
// mutable state is behind an interface, so extractState should report
// Kind=="interface" and surface the declared methods as state variables. The sim
// fixture's state is a struct, so this is the only exercise of the interfaceVars
// path. Lives outside testdata/sample. Never compiled — parsed only.
package ifacestate

// Store is the mutable state behind a contract; its methods are the state.
type Store interface {
	// Put writes val at key.
	Put(key string, val int) error
	// Delete removes key.
	Delete(key string) error
	// Get reads the value at key (read-only; surfaced faithfully regardless).
	Get(key string) (int, bool)
}
