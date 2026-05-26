// Pure aggregation: given the per-op and per-edge counts a DuckDB query
// returned, derive the per-node activity (for ScatterplotLayer radius) and
// per-edge activity (for PathLayer width) the flow view renders.
//
// Keeping this a pure function of (counts, model) means the deck.gl layer
// builders never touch DuckDB or React state — they take typed arrays in,
// produce GPU buffers out. That's what lets later stages drop in time-windowed
// queries without rewriting the renderer.

import type { DstModel, Edge } from "../types";
import { faultNodeId, invariantNodeId, opNodeId } from "../types";

export interface OpCount {
  op: number; // index into model.operations
  count: number;
}

// Per-edge event count, keyed by the same node-id pair the model.edges list
// already uses so the flow view can drive deck.gl PathLayer width directly.
export interface EdgeFlow {
  from: string;
  to: string;
  count: number;
}

export interface AggregateInput {
  /** Total events the window covers (denominator for share calculations). */
  total: number;
  /** Per-op event count rows from DuckDB. */
  ops: OpCount[];
  /** Per-fault-bit event counts (bit index → events that fired that fault). */
  faultBitCounts?: Map<number, number>;
  /** Per-invariant-bit event counts (fired + violated combined). */
  invariantBitCounts?: Map<number, number>;
}

export interface AggregateResult {
  /** Node id → events touching that node in the window. Zero for inactive nodes. */
  nodeActivity: Map<string, number>;
  /** Per edge in model.edges, the count that should drive its width. */
  edgeFlow: EdgeFlow[];
}

/**
 * Compute per-node and per-edge activity counts. Inactive nodes appear with
 * count 0 (rather than missing) so the renderer can still draw them at base
 * radius; an "unreachable" node visually distinguishes from an active one.
 */
export function aggregate(model: DstModel, input: AggregateInput): AggregateResult {
  const nodeActivity = new Map<string, number>();

  // Seed all known nodes at 0 so the result is total (every node id present).
  for (const op of model.operations) nodeActivity.set(opNodeId(op.index), 0);
  for (const f of model.faults) nodeActivity.set(faultNodeId(f.id), 0);
  for (const inv of model.invariants) nodeActivity.set(invariantNodeId(inv.id), 0);

  // Per-op activity: direct lookup.
  for (const { op, count } of input.ops) {
    const id = opNodeId(op);
    if (nodeActivity.has(id)) nodeActivity.set(id, count);
  }

  // Per-fault / per-invariant counts when the query provided them.
  if (input.faultBitCounts) {
    for (const [bit, count] of input.faultBitCounts) {
      const f = model.faults[bit];
      if (f) nodeActivity.set(faultNodeId(f.id), count);
    }
  }
  if (input.invariantBitCounts) {
    for (const [bit, count] of input.invariantBitCounts) {
      const inv = model.invariants[bit];
      if (inv) nodeActivity.set(invariantNodeId(inv.id), count);
    }
  }

  // Per-edge flow: approximate by min(from, to) activity — without a join
  // query the exact per-edge count is unknown, but the visual cue we need
  // (which edges are busy in this window) is preserved. Stage 4 replaces this
  // with a real edge-level DuckDB join.
  const edgeFlow: EdgeFlow[] = [];
  for (const e of model.edges) {
    const a = nodeActivity.get(e.from) ?? 0;
    const b = nodeActivity.get(e.to) ?? 0;
    edgeFlow.push({ from: e.from, to: e.to, count: Math.min(a, b) });
  }

  return { nodeActivity, edgeFlow };
}

/**
 * Edge weight bucketed for deck.gl: 1px base, up to 8px proportional to log
 * of the count. Log scaling keeps the busiest edges visible without flattening
 * the quieter ones; linear blew out on real DST runs where a few hot paths
 * dominate by 4+ orders of magnitude.
 */
export function edgeWidth(count: number): number {
  if (count <= 0) return 1;
  return 1 + Math.min(7, Math.log10(count + 1) * 2.2);
}

/**
 * Node radius scaling for the activity halo: base + log(count). Returns the
 * base radius when count is zero so inactive nodes still render.
 */
export function nodeRadius(baseRadius: number, count: number): number {
  if (count <= 0) return baseRadius;
  return baseRadius + Math.min(8, Math.log10(count + 1) * 2);
}

// Re-export Edge so callers can type their input without importing from ../types.
export type { Edge };
