import { describe, expect, it } from "vitest";
import type { DstModel } from "../types";
import { faultNodeId, invariantNodeId, opNodeId } from "../types";
import { aggregate, edgeWidth, nodeRadius } from "./aggregate";

const tinyModel: DstModel = {
  generatedAt: "2025-01-01T00:00:00Z",
  source: { simPath: "x", filesRead: 0, toolModule: "" },
  truths: [{ name: "PAYMENT", count: 2 }],
  invariants: [
    { id: "PAYMENT-1", label: "A", truth: "PAYMENT" },
    { id: "PAYMENT-2", label: "B", truth: "PAYMENT" },
  ],
  faults: [
    { id: "credit_race", enum: "FaultCreditRace", label: "credit race" },
    { id: "clock_skew", enum: "FaultClockSkew", label: "clock skew" },
  ],
  operations: [
    { index: 0, handler: "h0", name: "Payout", weight: 1, share: 0.5, faults: ["FaultCreditRace"] },
    { index: 1, handler: "h1", name: "Settle", weight: 1, share: 0.5, faults: [] },
  ],
  edges: [
    { from: opNodeId(0), to: faultNodeId("credit_race"), kind: "op-injects-fault" },
    { from: faultNodeId("credit_race"), to: invariantNodeId("PAYMENT-1"), kind: "mentions-invariant" },
  ],
  stats: {
    truths: 1, invariants: 2, faults: 2, operations: 2,
    totalWeight: 2, edges: 2,
    specsTotal: 0, validated: 0, uncheckedSpec: 0, unspecified: 0,
  },
};

describe("aggregate", () => {
  it("seeds every known node at zero", () => {
    const out = aggregate(tinyModel, { total: 0, ops: [] });
    expect(out.nodeActivity.get(opNodeId(0))).toBe(0);
    expect(out.nodeActivity.get(opNodeId(1))).toBe(0);
    expect(out.nodeActivity.get(faultNodeId("credit_race"))).toBe(0);
    expect(out.nodeActivity.get(invariantNodeId("PAYMENT-1"))).toBe(0);
  });

  it("attributes per-op counts to the right node id", () => {
    const out = aggregate(tinyModel, {
      total: 10,
      ops: [{ op: 0, count: 7 }, { op: 1, count: 3 }],
    });
    expect(out.nodeActivity.get(opNodeId(0))).toBe(7);
    expect(out.nodeActivity.get(opNodeId(1))).toBe(3);
  });

  it("attributes per-bit fault and invariant counts via the model order", () => {
    const out = aggregate(tinyModel, {
      total: 5,
      ops: [],
      faultBitCounts: new Map([[0, 4]]),       // bit 0 → faults[0] = credit_race
      invariantBitCounts: new Map([[1, 2]]),   // bit 1 → invariants[1] = PAYMENT-2
    });
    expect(out.nodeActivity.get(faultNodeId("credit_race"))).toBe(4);
    expect(out.nodeActivity.get(invariantNodeId("PAYMENT-2"))).toBe(2);
  });

  it("derives per-edge flow as min of endpoint activity", () => {
    const out = aggregate(tinyModel, {
      total: 10,
      ops: [{ op: 0, count: 7 }],
      faultBitCounts: new Map([[0, 3]]),
    });
    // op:0 (7) -> fault:credit_race (3) → edge count = 3
    expect(out.edgeFlow[0]).toEqual({
      from: opNodeId(0),
      to: faultNodeId("credit_race"),
      count: 3,
    });
  });
});

describe("edgeWidth / nodeRadius scaling", () => {
  it("returns base values for zero activity", () => {
    expect(edgeWidth(0)).toBe(1);
    expect(nodeRadius(7, 0)).toBe(7);
  });

  it("grows with log of count", () => {
    expect(edgeWidth(10)).toBeGreaterThan(edgeWidth(1));
    expect(nodeRadius(7, 1000)).toBeGreaterThan(nodeRadius(7, 10));
  });

  it("caps so a single hot path doesn't dominate the canvas", () => {
    expect(edgeWidth(1_000_000_000)).toBeLessThanOrEqual(8);
    expect(nodeRadius(7, 1_000_000_000)).toBeLessThanOrEqual(7 + 8);
  });
});
