import { describe, it, expect } from "vitest";
import { computeLayout } from "./graphLayout";

const nodes = ["op:0", "op:1", "fault:a", "inv:X", "inv:Y", "truth:T", "spec:S"];
const edges = [
  { from: "op:0", to: "fault:a" },
  { from: "truth:T", to: "inv:X" },
  { from: "truth:T", to: "inv:Y" },
  { from: "inv:X", to: "spec:S" },
  { from: "inv:Y", to: "inv:X" },
];

describe("computeLayout", () => {
  it("is deterministic — same input yields identical coordinates", () => {
    const a = computeLayout(nodes, edges);
    const b = computeLayout(nodes, edges);
    for (const id of nodes) {
      expect(b.pos.get(id)).toEqual(a.pos.get(id));
    }
  });

  it("places every node at a finite point inside the canvas", () => {
    const { pos, width, height } = computeLayout(nodes, edges, { width: 800, height: 600, margin: 20 });
    expect(pos.size).toBe(nodes.length);
    for (const id of nodes) {
      const p = pos.get(id)!;
      expect(Number.isFinite(p.x)).toBe(true);
      expect(Number.isFinite(p.y)).toBe(true);
      expect(p.x).toBeGreaterThanOrEqual(0);
      expect(p.x).toBeLessThanOrEqual(width);
      expect(p.y).toBeGreaterThanOrEqual(0);
      expect(p.y).toBeLessThanOrEqual(height);
    }
  });

  it("ignores edges to unknown nodes and self-edges without crashing", () => {
    const { pos } = computeLayout(["op:0", "op:1"], [
      { from: "op:0", to: "op:0" }, // self
      { from: "op:0", to: "ghost" }, // unknown endpoint
      { from: "op:0", to: "op:1" },
    ]);
    expect(pos.size).toBe(2);
  });

  it("handles empty and singleton inputs", () => {
    expect(computeLayout([], []).pos.size).toBe(0);
    const one = computeLayout(["op:0"], []);
    expect(one.pos.size).toBe(1);
    const p = one.pos.get("op:0")!;
    expect(Number.isFinite(p.x) && Number.isFinite(p.y)).toBe(true);
  });
});
