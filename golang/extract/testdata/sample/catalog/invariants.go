package catalog

// Invariant is one safety property the fixture sim checks; read by keyed field.
type Invariant struct {
	ID    string
	Label string
}

// Invariants is the keyed catalogue. The id prefix (ALPHA/BETA) is the "truth".
var Invariants = []Invariant{
	{ID: "ALPHA-1", Label: "NoNegativeBalance"},
	{ID: "ALPHA-2", Label: "ConservationOfValue"},
	{ID: "ALPHA-3", Label: "MonotonicLedger"},
	{ID: "BETA-1", Label: "UniqueShipmentIDs"},
	{ID: "BETA-2", Label: "NoDoubleComplete"},
	{ID: "BETA-3", Label: "OrderedDelivery"},
}
