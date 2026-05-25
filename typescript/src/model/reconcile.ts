// Spec ↔ implementation reconciliation: the cheapest, highest-leverage check for
// keeping a TLA+ corpus and its Go checkers honest (Stage 1 of the trace-
// validation roadmap — see "Validating Traces of Distributed Programs Against
// TLA+ Specifications", Cirstea/Kuppe/Loillier/Merz, SEFM 2024, arXiv:2404.16075).
//
// Three sources should agree on invariant names: the .tla modules (declared),
// the TLC .cfg (actually model-checked), and the Go checkers (enforced at
// runtime). The Go extractor already classified every invariant via specStatus
// and every spec invariant via `checked`, so the three drift sets fall straight
// out of the model — no re-parsing required.

import type { DstModel, Invariant, SpecInvariant } from "../types";

export interface ReconcileReport {
  /**
   * (1) Spec rot: declared in a .tla module but absent from the model-checked
   * .cfg set. A formal claim TLC is not actually verifying. Often expected
   * (state-space explosion forces a checked subset), so informational by default.
   */
  specRot: SpecInvariant[];
  /**
   * (2) Runtime gap: model-checked in TLA+ but with no Go checker bound to it.
   * The sharpest drift — DST will not catch a regression of a property the spec
   * proves. This is what `--strict` gates on.
   */
  runtimeGap: SpecInvariant[];
  /**
   * (3) Folk wisdom: enforced by a Go checker with no TLA+ counterpart. Either
   * promote it to the spec, or knowingly accept it as sim-only (e.g. ECONOMICS).
   */
  folkWisdom: Invariant[];

  /**
   * (4) Drafts: machine-generated spec invariants (`astdst generate`) that TLC
   * has not verified clean. A `⚙✗` is a draft, not a checked claim — informational,
   * not blocking.
   */
  generatedUnverified: SpecInvariant[];

  goInvariants: number;
  specInvariants: number;
  validated: number; // Go checkers bound to a model-checked spec invariant
  generatedTotal: number; // spec invariants that are machine-generated
  generatedVerified: number; // generated AND TLC-verified against real value transitions
}

// TLA+ convention: TypeOK / a *TypeInvariant is the type-correctness predicate.
// It is routinely model-checked but is not a domain safety property a runtime
// checker would mirror, so it is not a real "runtime gap".
const isTypeInvariant = (name: string): boolean =>
  /^TypeOK/i.test(name) || /TypeInvariant/i.test(name);

export function reconcile(model: DstModel): ReconcileReport {
  const specs = model.specs ?? [];
  const linked = new Set<string>();
  for (const inv of model.invariants) {
    if (inv.spec) linked.add(inv.spec.name);
  }

  const generated = specs.filter((s) => s.generated);
  return {
    specRot: specs.filter((s) => !s.checked),
    runtimeGap: specs.filter(
      (s) => s.checked && !linked.has(s.name) && !isTypeInvariant(s.name),
    ),
    folkWisdom: model.invariants.filter((i) => i.specStatus === "unspecified"),
    generatedUnverified: generated.filter((s) => !s.verified),
    goInvariants: model.invariants.length,
    specInvariants: specs.length,
    validated: model.invariants.filter((i) => i.specStatus === "validated").length,
    generatedTotal: generated.length,
    generatedVerified: generated.filter((s) => s.verified && s.behavioral).length,
  };
}

/** True when the report contains drift worth failing CI over (the runtime gap). */
export function hasBlockingDrift(r: ReconcileReport): boolean {
  return r.runtimeGap.length > 0;
}
