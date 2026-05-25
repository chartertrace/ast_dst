import type { SpecRef, SpecStatus } from "../types";

/** Shared row className builder for the selectable list rows. */
export function rowClass(selected: boolean, dim: boolean): string {
  return ["dstast-row", selected ? "selected" : "", dim ? "dim" : ""]
    .filter(Boolean)
    .join(" ");
}

/** How a spec-coverage status renders: glyph, CSS modifier, and tooltip. */
export interface SpecBadge {
  glyph: string;
  cls: SpecStatus;
  title: string;
}

export function specBadge(status: SpecStatus | undefined): SpecBadge | null {
  switch (status) {
    case "validated":
      return { glyph: "✓", cls: status, title: "Model-checked in TLA+ and exercised by a runtime Go checker — sampling via DST, not a refinement proof of the code" };
    case "unchecked-spec":
      return { glyph: "⚠", cls: status, title: "Specified in TLA+ but not in the model-checked .cfg set" };
    case "unspecified":
      return { glyph: "○", cls: status, title: "No TLA+ specification — validated only at runtime" };
    default:
      return null; // no TLA+ layer configured
  }
}

/** Provenance badge for a machine-generated spec, distinct from coverage. */
export interface GenBadge {
  glyph: string;
  cls: "gen-verified" | "gen-wellformed" | "gen-unverified";
  title: string;
}

// Three honest tiers: behavioral (TLC checked an active invariant against real
// value transitions), well-formed (TLC passed but transitions carry no value
// semantics — a weak claim), and unverified.
export function genBadge(spec: SpecRef | undefined): GenBadge | null {
  if (!spec?.generated) return null; // hand-written spec, or no spec
  if (!spec.verified) {
    return { glyph: "⚙✗", cls: "gen-unverified", title: "Machine-generated but NOT verified by TLC — treat as a draft" };
  }
  if (spec.behavioral) {
    return { glyph: "⚙✓", cls: "gen-verified", title: "Machine-generated; TLC checked this invariant against real value transitions (the abstract model — not a proof the Go code conforms)" };
  }
  return { glyph: "⚙~", cls: "gen-wellformed", title: "Machine-generated and TLC-parseable, but the transitions carry no value semantics — a weak claim, not behavioral verification" };
}
