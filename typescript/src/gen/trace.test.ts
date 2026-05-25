import { describe, expect, it } from "vitest";
import type { DstModel } from "../types";
import { generateTraceSpec } from "./trace";
import sample from "../../sample/model.json";

const model = sample as unknown as DstModel;

describe("generateTraceSpec", () => {
  const { module, cfg, moduleName } = generateTraceSpec(model, { baseModule: "PayoutFlowGen" });

  it("extends the given (Go-generated) base module and wires the trace machinery", () => {
    expect(moduleName).toBe("PayoutFlowGenTrace");
    expect(module).toContain("---- MODULE PayoutFlowGenTrace ----");
    expect(module).toContain("EXTENDS PayoutFlowGen, TLC");
    expect(module).toContain("ndJsonDeserialize(IOEnv.TRACE_PATH)");
    expect(module).toContain("IsEvent(e) ==");
    expect(module).toContain('TraceAccepted == Len(Trace) = TLCGet("stats").diameter - 1');
  });

  it("composes one event per operation with its spec action via \\cdot", () => {
    for (const op of model.operations) {
      expect(module).toContain(`IsEvent("${op.name}") \\cdot`);
    }
    const disjuncts = (module.match(/IsEvent\("[^"]+"\) \\cdot/g) ?? []).length;
    expect(disjuncts).toBe(model.operations.length);
  });

  it("defaults the base module to DstSpec", () => {
    expect(generateTraceSpec(model).moduleName).toBe("DstSpecTrace");
  });

  it("writes a trace config with the acceptance postcondition", () => {
    expect(cfg).toContain("SPECIFICATION TraceSpec");
    expect(cfg).toContain("POSTCONDITION TraceAccepted");
  });
});
