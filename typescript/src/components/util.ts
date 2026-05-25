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
  cls: "gen-verified" | "gen-unverified";
  title: string;
}

export function genBadge(spec: SpecRef | undefined): GenBadge | null {
  if (!spec?.generated) return null; // hand-written spec, or no spec
  return spec.verified
    ? { glyph: "⚙✓", cls: "gen-verified", title: "Machine-generated; TLC verified the abstract model — not that the Go code conforms" }
    : { glyph: "⚙✗", cls: "gen-unverified", title: "Machine-generated but NOT verified by TLC — treat as a draft" };
}
