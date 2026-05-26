// SQL helpers for the flow view. Stage 1: whole-run per-op counts. Stages 2-5
// extend with time windows, bit-level fault/invariant counts, and brushed
// causal joins — keeping queries in one module makes that diff easy to read.

import type { AsyncDuckDBConnection } from "@duckdb/duckdb-wasm";
import type { OpCount } from "./aggregate";

export interface OpCountResult {
  total: number;
  ops: OpCount[];
}

/**
 * Per-op event counts across the entire trace. DuckDB returns Arrow under the
 * hood, so values come back as the right widths (counts are bigint until we
 * narrow them; op fits in a uint16 → number).
 */
export async function queryOpCounts(conn: AsyncDuckDBConnection): Promise<OpCountResult> {
  const res = await conn.query(
    `SELECT op, COUNT(*) AS cnt FROM trace GROUP BY op ORDER BY op`,
  );
  const ops: OpCount[] = [];
  let total = 0;
  for (const row of res.toArray()) {
    // Arrow JS surfaces row fields via property access; widths come back as
    // the underlying type (count is bigint from COUNT()).
    const r = row as unknown as { op: number; cnt: bigint };
    const count = Number(r.cnt);
    ops.push({ op: Number(r.op), count });
    total += count;
  }
  return { total, ops };
}
