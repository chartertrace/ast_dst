package engine

import "github.com/chartertrace/ast_dst/golang/extract/testdata/sample/catalog"

// recordFault is the fixture's instrumentation hook — the call the extractor
// follows from each operation handler to derive the op->fault edges.
func recordFault(f catalog.FaultType) { _ = f }

// opTable is the weighted dispatch table, in index order.
var opTable = [4]func(){
	opCreateShipment,
	opCompleteShipment,
	opTransfer,
	opTick,
}

var opWeights [4]float64

func initWeights() {
	opWeights = [4]float64{
		5,  // 0: CreateShipment - need entities to work with
		3,  // 1: CompleteShipment
		2,  // 2: Transfer
		10, // 3: Tick - advance the simulated clock
	}
}

// w is the engine's mutable state, the instance the handlers act on.
var w World

// opCreateShipment registers a shipment and can lose a concurrent credit.
func opCreateShipment() {
	w.Shipments["s"] = "created"
	recordFault(catalog.FaultCreditRace)
}

// opCompleteShipment marks a shipment done; may crash once the write is durable.
func opCompleteShipment() {
	w.Shipments["s"] = "completed"
	recordFault(catalog.FaultCrashAfterWrite)
}

// opTransfer moves a balance and can fail the transfer or skew the clock.
func opTransfer() {
	w.Balances["d"] += 1
	recordFault(catalog.FaultTransferFail)
	recordFault(catalog.FaultClockSkew)
}

// opTick advances the simulated clock and injects no fault.
func opTick() { w.Clock++ }
