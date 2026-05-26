// Package embed is a fixture for embedded-struct flattening in state extraction:
// it composes its state from a local embed, a pointer embed, and an external
// embed, plus a field that shadows a promoted one.
package embed

import "sync"

// Meta is embedded by value into World; its fields are promoted.
type Meta struct {
	Version int    // promoted to World as Version (via Meta)
	Label   string // shadowed by World.Label, so not promoted
}

// Audit is embedded by pointer into World; its fields are promoted too.
type Audit struct {
	Count int // promoted to World as Count (via Audit)
}

// World is the fixture's mutable state, built by composition.
type World struct {
	sync.Mutex        // external embed: unresolvable under go/ast, kept opaque
	Meta              // local value embed: flattened
	*Audit            // local pointer embed: flattened
	Clock      int64  // direct field
	Label      string // direct field; shadows the promoted Meta.Label
}
