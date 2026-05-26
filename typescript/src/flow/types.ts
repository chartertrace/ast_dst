// Wire-format mirror of golang/trace/trace.go's Event struct. Kept in lockstep
// with the Go side; both consume the same NDJSON lines. The Parquet file the
// flow viewer queries is a denormalised projection (op resolved to uint16,
// faults/invariants as bitsets) — that schema lives in schema.ts.

export interface TraceEvent {
  i: number;
  t: number;
  op: string;
  writes?: Record<string, unknown>;
  faults?: string[];
  invariants?: {
    fired?: string[];
    violated?: string[];
  };
}

// One row as DuckDB returns it from the Parquet file. Bitsets stay as bigint
// because uint64 doesn't fit in a JS number; DuckDB's Arrow output preserves
// 64-bit width. Bit positions index into model.faults / model.invariants.
export interface TraceRow {
  i: bigint;
  t: bigint;
  op: number;
  fault_bits: bigint;
  inv_fired_bits: bigint;
  inv_violated_bits: bigint;
  writes_json: Uint8Array | null;
}
