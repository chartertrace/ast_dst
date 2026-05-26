import { describe, expect, it } from "vitest";
import { TRACE_SCHEMA_CANONICAL, TRACE_SCHEMA_HASH, traceSchemaHash } from "./schema";

describe("trace parquet schema", () => {
  it("hashes the canonical descriptor to the pinned value", async () => {
    // If this fails: see the directions in schema.ts to update both Go and TS
    // constants together. Drift here means the writer and the reader disagree
    // on the on-disk shape — fail loudly rather than parse garbage at runtime.
    expect(await traceSchemaHash()).toBe(TRACE_SCHEMA_HASH);
  });

  it("declares one nullable column (writes_json)", () => {
    const cols = TRACE_SCHEMA_CANONICAL.split(";");
    const nullable = cols.filter((c) => c.endsWith("?"));
    expect(nullable).toEqual(["writes_json:binary?"]);
  });
});
