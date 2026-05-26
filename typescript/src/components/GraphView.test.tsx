import { describe, it, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { GraphView } from "./GraphView";
import type { DstModel } from "../types";

// Minimal model touching all five node kinds and every edge kind.
const model = {
  generatedAt: "2026-01-01T00:00:00Z",
  source: { simPath: "x", filesRead: 1, toolModule: "astdst" },
  truths: [{ name: "T", count: 2 }],
  invariants: [
    { id: "X", label: "InvX", truth: "T" },
    { id: "Y", label: "InvY", truth: "T" },
  ],
  faults: [{ id: "a", enum: "FaultA", label: "Fault A" }],
  operations: [
    { index: 0, handler: "opTick", name: "Tick", weight: 1, share: 1, faults: ["FaultA"] },
    { index: 1, handler: "opStep", name: "Step", weight: 1, share: 1, faults: [] },
  ],
  specs: [{ name: "S", predicate: "TRUE", specFile: "f.tla", checked: true }],
  edges: [
    { from: "op:0", to: "fault:a", kind: "op-injects-fault" },
    { from: "truth:T", to: "inv:X", kind: "truth-has-invariant" },
    { from: "truth:T", to: "inv:Y", kind: "truth-has-invariant" },
    { from: "inv:X", to: "spec:S", kind: "validates" },
    { from: "inv:Y", to: "inv:X", kind: "mentions-invariant" },
  ],
  stats: {
    truths: 1, invariants: 2, faults: 1, operations: 2, totalWeight: 2,
    edges: 5, specsTotal: 1, validated: 1, uncheckedSpec: 0, unspecified: 1,
  },
} as unknown as DstModel;

const render = (selected: string | null, highlight: Set<string>) =>
  renderToStaticMarkup(
    <GraphView model={model} selected={selected} highlight={highlight} onSelect={() => {}} onClear={() => {}} />,
  );

describe("GraphView", () => {
  it("draws one node per id and one line per edge", () => {
    const html = render(null, new Set());
    // 7 nodes: 2 ops + 1 fault + 2 invariants + 1 truth + 1 spec.
    expect((html.match(/<circle/g) ?? []).length).toBe(7);
    // 5 edges, all with resolvable endpoints.
    expect((html.match(/<line/g) ?? []).length).toBe(5);
    expect(html).toContain("dstast-graph-svg");
  });

  it("labels truths by default, not every node", () => {
    const html = render(null, new Set());
    expect(html).toContain(">T<"); // truth labelled
    expect(html).not.toContain(">Tick<"); // op label hidden until focus/hover
    expect(html).not.toContain(">InvX<"); // invariant label hidden until focus/hover
  });

  it("uses kind-specific classes so the palette applies", () => {
    const html = render(null, new Set());
    for (const k of ["op", "fault", "inv", "truth", "spec"]) {
      expect(html).toContain(`dstast-gnode-${k}`);
    }
    expect(html).toContain("dstast-gedge-validates");
  });

  it("labels the svg and every node for assistive tech", () => {
    const html = render(null, new Set());
    expect(html).toMatch(/aria-label="Node-link graph: 7 nodes, 5 edges[^"]*"/);
    expect(html).toContain('aria-label="operation · Tick"');
    expect(html).toContain('role="button"'); // nodes are announced as actionable
  });
});
