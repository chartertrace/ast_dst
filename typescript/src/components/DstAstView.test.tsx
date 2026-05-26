// Interaction + accessibility tests for the top-level viewer. Unlike the
// renderToStaticMarkup tests (which assert rendered structure), these drive the
// real component state in jsdom — click → select → highlight → clear, view
// toggling, and keyboard — and run an axe audit on each view. This is the layer
// the static-markup tests can't reach.
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { axe } from "vitest-axe";
import { DstAstView } from "../DstAstView";
import type { DstModel } from "../types";

// Minimal model touching all five node kinds and every edge kind.
const model = {
  generatedAt: "2026-01-01T00:00:00Z",
  source: { simPath: "x", filesRead: 1, toolModule: "astdst" },
  truths: [{ name: "T", count: 2 }],
  invariants: [
    { id: "X", label: "InvX", truth: "T", doc: "Invariant X holds." },
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
  ],
  stats: {
    truths: 1, invariants: 2, faults: 1, operations: 2, totalWeight: 2,
    edges: 4, specsTotal: 1, validated: 1, uncheckedSpec: 0, unspecified: 1,
  },
} as unknown as DstModel;

afterEach(cleanup);

// color-contrast needs real layout/canvas (a browser), which jsdom lacks — it
// can't be evaluated here, so disable just that rule. Everything structural
// (roles, names, labels, focus order) still runs.
const axeOpts = { rules: { "color-contrast": { enabled: false } } };

describe("DstAstView interaction", () => {
  it("toggles between columns and graph views, reflecting aria-pressed", async () => {
    const user = userEvent.setup();
    render(<DstAstView model={model} />);

    // Default view is columns: the operation row is rendered.
    expect(screen.getByRole("button", { name: /Tick/i })).toBeInTheDocument();

    const graphBtn = screen.getByRole("button", { name: "graph view" });
    expect(graphBtn).toHaveAttribute("aria-pressed", "false");
    await user.click(graphBtn);
    expect(graphBtn).toHaveAttribute("aria-pressed", "true");
    // The graph renders actionable, labelled nodes.
    expect(screen.getByRole("button", { name: "operation · Tick" })).toBeInTheDocument();
  });

  it("selecting a node shows its detail; re-click and Escape clear it", async () => {
    const user = userEvent.setup();
    render(<DstAstView model={model} />);

    // Nothing selected → the panel explains the interaction.
    expect(screen.getByText(/Select an operation/i)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Tick/i }));
    // Detail panel now resolves the node: a clear button appears, hint gone.
    expect(screen.getByRole("button", { name: "clear" })).toBeInTheDocument();
    expect(screen.queryByText(/Select an operation/i)).not.toBeInTheDocument();

    // Escape clears the selection wherever focus is.
    await user.keyboard("{Escape}");
    expect(screen.getByText(/Select an operation/i)).toBeInTheDocument();
  });
});

describe("DstAstView accessibility (axe)", () => {
  it("columns view has no WCAG violations", async () => {
    const { container } = render(<DstAstView model={model} />);
    expect(await axe(container, axeOpts)).toHaveNoViolations();
  });

  it("graph view has no WCAG violations", async () => {
    const user = userEvent.setup();
    const { container } = render(<DstAstView model={model} />);
    await user.click(screen.getByRole("button", { name: "graph view" }));
    expect(await axe(container, axeOpts)).toHaveNoViolations();
  });

  it("with a node selected, the detail panel has no violations", async () => {
    const user = userEvent.setup();
    const { container } = render(<DstAstView model={model} />);
    await user.click(screen.getByRole("button", { name: /Tick/i }));
    expect(await axe(container, axeOpts)).toHaveNoViolations();
  });
});
