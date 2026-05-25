// Resolve the formal predicate for an invariant, with provenance. Shared by the
// TLA+ generator and the doc-site so both surface the exact same predicate text.
//
// Precedence: the verbatim ASCII predicate from a linked TLA+ spec wins; failing
// that, the predicate embedded (in Unicode glyphs) in the doc comment, converted
// to ASCII; failing that, none — callers render a stub rather than invent one.

import type { Invariant, SpecInvariant } from "../types";
import { extractDocPredicate } from "./glyphs";

export type PredicateSource = "spec" | "doc" | "none";

export interface ResolvedPredicate {
  text: string; // ASCII predicate; "" when source is "none"
  source: PredicateSource;
}

export function invariantPredicate(
  inv: Invariant,
  specsByName: Map<string, SpecInvariant>,
): ResolvedPredicate {
  if (inv.spec) {
    const spec = specsByName.get(inv.spec.name);
    if (spec?.predicate?.trim()) return { text: spec.predicate.trim(), source: "spec" };
  }
  const doc = extractDocPredicate(inv.doc);
  if (doc) return { text: doc, source: "doc" };
  return { text: "", source: "none" };
}

/** Index a model's spec invariants by name, for repeated lookups. */
export function specsByName(specs: SpecInvariant[] | undefined): Map<string, SpecInvariant> {
  return new Map((specs ?? []).map((s) => [s.name, s] as const));
}
