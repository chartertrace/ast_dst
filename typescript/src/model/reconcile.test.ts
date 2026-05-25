import { describe, expect, it } from "vitest";
import type { DstModel } from "../types";
import { hasBlockingDrift, reconcile } from "./reconcile";
import sample from "../../sample/model.json";
import generatedSample from "../../sample/model.generated.json";

const model = sample as unknown as DstModel;
const generated = generatedSample as unknown as DstModel;

describe("reconcile", () => {
  const r = reconcile(model);

  it("partitions invariants into the three drift sets without overlap of counts", () => {
    // validated + sim-only(folkWisdom) accounts for the Go invariants that are
    // either bound-and-checked or unspecified; totals come straight from the model.
    expect(r.goInvariants).toBe(model.invariants.length);
    expect(r.specInvariants).toBe((model.specs ?? []).length);
    expect(r.validated).toBe(model.stats.validated);
    expect(r.folkWisdom.length).toBe(model.stats.unspecified);
  });

  it("runtime gaps are model-checked specs with no Go checker", () => {
    const linked = new Set(model.invariants.flatMap((i) => (i.spec ? [i.spec.name] : [])));
    for (const s of r.runtimeGap) {
      expect(s.checked).toBe(true);
      expect(linked.has(s.name)).toBe(false);
    }
  });

  it("spec rot are declared-but-unchecked specs", () => {
    for (const s of r.specRot) expect(s.checked).toBe(false);
  });

  it("hasBlockingDrift tracks the runtime gap", () => {
    expect(hasBlockingDrift(r)).toBe(r.runtimeGap.length > 0);
  });
});

describe("reconcile — machine-generated provenance", () => {
  const r = reconcile(generated);

  it("counts generated specs and the TLC-verified (behavioral) subset", () => {
    expect(r.generatedTotal).toBe(3);
    expect(r.generatedVerified).toBe(1); // NoNegativeBalance: verified + behavioral
  });

  it("flags generated-but-unverified drafts", () => {
    expect(r.generatedUnverified.map((s) => s.name)).toEqual(["FrozenImpliesEmpty"]);
  });

  it("a verified-but-not-behavioral spec is a runtime gap, not 'verified'", () => {
    // WellFormedOnly is checked + generated + verified but behavioral=false, and no
    // Go checker binds to it -> it surfaces as a runtime gap and not in generatedVerified.
    expect(r.runtimeGap.map((s) => s.name)).toContain("WellFormedOnly");
  });
});
