import { describe, it, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { OperationList } from "./OperationList";
import type { Fault, Operation } from "../types";

const operations: Operation[] = [
  { index: 0, handler: "opTick", name: "Tick", weight: 2, share: 0.5, faults: ["FaultA"] },
  { index: 1, handler: "opStep", name: "Step", weight: 2, share: 0.5, faults: [] },
];
const faultByEnum = new Map<string, Fault>([["FaultA", { id: "a", enum: "FaultA", label: "Fault A" }]]);

const render = (selected: string | null, highlight = new Set<string>()) =>
  renderToStaticMarkup(
    <OperationList
      operations={operations}
      faultByEnum={faultByEnum}
      label="Operations"
      selected={selected}
      highlight={highlight}
      onSelect={() => {}}
    />,
  );

describe("OperationList accessibility", () => {
  it("exposes selection state via aria-pressed (not just a CSS class)", () => {
    const none = render(null);
    expect(none).not.toContain('aria-pressed="true"');
    expect((none.match(/aria-pressed="false"/g) ?? []).length).toBe(2);

    const sel = render("op:0");
    // Exactly the selected row reports pressed.
    expect((sel.match(/aria-pressed="true"/g) ?? []).length).toBe(1);
    expect((sel.match(/aria-pressed="false"/g) ?? []).length).toBe(1);
  });

  it("gives each row a descriptive accessible name", () => {
    const html = render(null);
    expect(html).toContain('aria-label="operation Tick"');
    expect(html).toContain('aria-label="operation Step"');
  });

  it("renders selectable rows as real buttons (keyboard-activatable)", () => {
    const html = render(null);
    expect((html.match(/<button/g) ?? []).length).toBe(2);
  });
});
