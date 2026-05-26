// Shared CLI helpers for the model.json generators. The point is developer
// experience: a missing file, bad JSON, or the wrong file shape should produce a
// one-line, actionable message — not a raw Node stack trace.

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import type { DstModel } from "../types";

/** Print a friendly one-line error to stderr and exit non-zero. */
export function die(message: string): never {
  process.stderr.write(`error: ${message}\n`);
  process.exit(1);
}

/**
 * Read and parse a model.json, mapping the common failures to guidance:
 *   - missing file  → "run the Go extractor first"
 *   - invalid JSON  → which file and why
 *   - wrong shape   → "doesn't look like a model.json"
 */
export function loadModel(path: string): DstModel {
  const abs = resolve(path);

  let raw: string;
  try {
    raw = readFileSync(abs, "utf8");
  } catch (e) {
    if ((e as NodeJS.ErrnoException).code === "ENOENT") {
      die(`no model at ${abs}\n  Run the Go extractor first, e.g.  astdst --root <go repo> --out ${path}`);
    }
    die(`could not read ${abs}: ${(e as Error).message}`);
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch (e) {
    die(`${abs} is not valid JSON: ${(e as Error).message}`);
  }

  if (!parsed || typeof parsed !== "object" || !Array.isArray((parsed as DstModel).operations)) {
    die(`${abs} doesn't look like a model.json (no "operations" array). Point --in at the extractor's output.`);
  }
  return parsed as DstModel;
}

/** Run a CLI's main, turning any unexpected throw into a clean error + exit 1. */
export function runCli(main: () => void): void {
  try {
    main();
  } catch (e) {
    die((e as Error).message ?? String(e));
  }
}
