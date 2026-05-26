// Package cataloguefaults is a fixture for the catalogue-only fault path: a
// positional `[]struct{id, label}` fault catalogue with NO enum type and NO name
// map — the inverse of the enum-only fixture. The sim fixture always pairs a
// catalogue with an enum + faultNames, so this exercises the case where the
// slice alone drives id, label, and order. Lives outside testdata/sample so the
// recursive-discovery tests are unaffected. Never compiled — parsed only.
package cataloguefaults

// FaultRow is one positional catalogue row: {id, label}.
type FaultRow struct {
	id    string
	label string
}

// Faults is the fault catalogue in wire order; read positionally (0=id, 1=label).
var Faults = []FaultRow{
	{"disk_full", "Disk reached capacity"},
	{"net_partition", "Network partition between nodes"},
}
