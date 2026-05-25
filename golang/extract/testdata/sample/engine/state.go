package engine

// World is the fixture's mutable state — the variables the operations act on.
// The state extractor reads its fields as TLA+ VARIABLES candidates.
type World struct {
	// Clock is the simulated logical time, advanced by opTick.
	Clock int64
	// Shipments maps a shipment id to its status string.
	Shipments map[string]string
	// Balances is the per-driver balance in cents.
	Balances map[string]int64
	pending  []string // unexported: transfers awaiting settlement
}
