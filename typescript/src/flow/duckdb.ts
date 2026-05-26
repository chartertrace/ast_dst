// DuckDB-WASM singleton for the flow view. The viewer attaches a Parquet
// trace file once per session; subsequent queries reuse the same connection.
//
// Bundle loading uses the jsDelivr-hosted assets so consumers don't have to
// configure Vite asset handling. For air-gapped deploys, a future option will
// let callers pass their own DuckDBBundle.

import * as duckdb from "@duckdb/duckdb-wasm";

let initPromise: Promise<duckdb.AsyncDuckDB> | null = null;

async function init(): Promise<duckdb.AsyncDuckDB> {
  const bundles = duckdb.getJsDelivrBundles();
  const bundle = await duckdb.selectBundle(bundles);
  const workerSrc = `importScripts("${bundle.mainWorker!}");`;
  const workerURL = URL.createObjectURL(new Blob([workerSrc], { type: "text/javascript" }));
  const worker = new Worker(workerURL);
  const logger = new duckdb.ConsoleLogger(duckdb.LogLevel.WARNING);
  const db = new duckdb.AsyncDuckDB(logger, worker);
  await db.instantiate(bundle.mainModule, bundle.pthreadWorker);
  URL.revokeObjectURL(workerURL);
  return db;
}

/** Returns a process-wide DuckDB-WASM instance, initialising on first call. */
export function getDuckDB(): Promise<duckdb.AsyncDuckDB> {
  if (!initPromise) initPromise = init();
  return initPromise;
}

/**
 * Attach a Parquet file at the given URL as a SQL view named `trace`. The URL
 * is fetched once into DuckDB's virtual FS; subsequent queries are local.
 * Range-request streaming (HTTPFS) is a follow-up — for Stage 1 we read the
 * whole file, which is fine for traces that fit in browser memory.
 */
export async function attachTrace(url: string): Promise<duckdb.AsyncDuckDBConnection> {
  const db = await getDuckDB();
  const res = await fetch(url);
  if (!res.ok) throw new Error(`flow: fetch ${url}: ${res.status} ${res.statusText}`);
  const buf = new Uint8Array(await res.arrayBuffer());
  await db.registerFileBuffer("trace.parquet", buf);
  const conn = await db.connect();
  await conn.query(`CREATE OR REPLACE VIEW trace AS SELECT * FROM read_parquet('trace.parquet')`);
  return conn;
}
