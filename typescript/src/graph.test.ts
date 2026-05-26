import { describe, it, expect } from "vitest";
import { buildGraph, highlightSet } from "./graph";
import type { DstModel } from "./types";

// `structure` mode emits operations with faults: null; buildGraph must tolerate
// that (it once threw "faults is not iterable", blanking the whole viewer).
const model = {
  generatedAt: "2026-01-01T00:00:00Z",
  source: { simPath: "x", filesRead: 1, toolModule: "astdst" },
  truths: [{ name: "pkg", count: 1 }],
  invariants: [{ id: "G", label: "Global", truth: "pkg" }],
  faults: [{ id: "t", enum: "TypeT", label: "Type T" }],
  operations: [
    { index: 0, handler: "F", name: "F", weight: 1, share: 1, faults: null },
    { index: 1, handler: "G", name: "G", weight: 1, share: 1, faults: ["TypeT"] },
  ],
  edges: [
    { from: "truth:pkg", to: "inv:G", kind: "truth-has-invariant" },
    { from: "op:1", to: "fault:t", kind: "op-injects-fault" },
  ],
  stats: {
    truths: 1, invariants: 1, faults: 1, operations: 2, totalWeight: 2,
    edges: 2, specsTotal: 0, validated: 0, uncheckedSpec: 0, unspecified: 1,
  },
} as unknown as DstModel;

describe("buildGraph", () => {
  it("tolerates operations with null faults", () => {
    const g = buildGraph(model);
    expect(g.opsByFaultId.get("t")?.length).toBe(1); // op:1 injects it
    expect(g.opsByFaultId.has("nonexistent")).toBe(false);
  });

  it("links edges undirected and resolves one-hop neighbourhoods", () => {
    const g = buildGraph(model);
    const hl = highlightSet(g, "fault:t");
    expect(hl.has("fault:t")).toBe(true);
    expect(hl.has("op:1")).toBe(true); // neighbour via op-injects-fault
    expect(highlightSet(g, null).size).toBe(0);
  });
});
