import { describe, expect, it } from "vitest";
import type { DstModel } from "../types";
import { generateTla, generateTraceSpec } from "./tla";
import sample from "../../sample/model.json";

const model = sample as unknown as DstModel;

describe("generateTla", () => {
  const { module, cfg, report } = generateTla(model, { moduleName: "Test" });

  it("emits a structurally complete module", () => {
    expect(module).toContain("---- MODULE Test ----");
    expect(module).toContain("EXTENDS");
    expect(module).toContain("Init == TRUE");
    expect(module).toContain("Next ==");
    expect(module).toContain("Spec == Init /\\ [][Next]_vars");
    expect(module).toContain("AllInvariants ==");
    expect(module).toMatch(/====\s*$/);
  });

  it("covers every invariant and operation", () => {
    expect(report.invariants).toBe(model.invariants.length);
    expect(report.operations).toBe(model.operations.length);
    expect(report.fromSpec + report.fromDoc + report.docNeedsHand + report.stubbed).toBe(
      model.invariants.length,
    );
    // The sample has validated invariants, so at least one real spec predicate.
    expect(report.fromSpec).toBeGreaterThan(0);
  });

  it("emits ASCII-only TLA+ in the active body (non-comment lines)", () => {
    // Comments may quote recovered non-TLA+ math for hand-transcription; the live
    // spec body must be pure ASCII or SANY rejects it. Strip full-line comments.
    const active = module
      .split("\n")
      .filter((l) => !l.trim().startsWith("\\*"))
      .join("\n");
    // eslint-disable-next-line no-control-regex
    expect(active).not.toMatch(/[^\x00-\x7F]/); // no non-ASCII anywhere live
    expect(active).not.toMatch(/\|\s*[{(]/); // no |S| cardinality bars live
    expect(active).toContain("\\A"); // ASCII quantifiers did survive
  });

  it("writes a config that model-checks the aggregate invariant", () => {
    expect(cfg).toContain("SPECIFICATION Spec");
    expect(cfg).toContain("INVARIANT AllInvariants");
  });
});

describe("generateTraceSpec", () => {
  const base = generateTla(model, { moduleName: "Test" });
  const { module, cfg } = generateTraceSpec(model, {
    baseModule: base.moduleName,
    opNames: base.opNames,
  });

  it("extends the base module and wires the trace machinery", () => {
    expect(module).toContain("---- MODULE TestTrace ----");
    expect(module).toContain("EXTENDS Test, TLC");
    expect(module).toContain("ndJsonDeserialize(IOEnv.TRACE_PATH)");
    expect(module).toContain("IsEvent(e) ==");
    expect(module).toContain("TraceAccepted == Len(Trace) = TLCGet(\"stats\").diameter - 1");
  });

  it("composes one event per operation with its spec action via \\cdot", () => {
    for (const op of model.operations) {
      expect(module).toContain(`IsEvent("${op.name}") \\cdot`);
    }
    const disjuncts = (module.match(/IsEvent\("[^"]+"\) \\cdot/g) ?? []).length;
    expect(disjuncts).toBe(model.operations.length);
  });

  it("writes a trace config with the acceptance postcondition", () => {
    expect(cfg).toContain("SPECIFICATION TraceSpec");
    expect(cfg).toContain("POSTCONDITION TraceAccepted");
  });
});
