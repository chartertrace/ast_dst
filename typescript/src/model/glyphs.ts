// TLA+ source is ASCII. Several invariant doc comments in the Go sim quote their
// formal spec using Unicode math glyphs (∀ ∈ ≤ ⇒ …) for readability. When we
// generate a *.tla scaffold we must emit the ASCII forms the TLA+ tools accept.
// This module converts glyphs → ASCII and harvests the embedded predicate from a
// doc comment. Both are pure and dependency-free, matching the rest of the package.

// Ordered longest-key-first so multi-codepoint glyphs win over their prefixes.
// Each entry is [glyph, ASCII]. The ASCII forms are the standard TLA+ operators
// (see Lamport, "Specifying Systems", §15.1.1).
const GLYPHS: ReadonlyArray<readonly [string, string]> = [
  ["⟹", "=>"],
  ["⟺", "<=>"],
  ["⇔", "<=>"],
  ["⇒", "=>"],
  ["≤", "<="],
  ["≥", ">="],
  ["≠", "#"],
  ["≜", "=="],
  ["≡", "<=>"],
  ["∀", "\\A"],
  ["∃", "\\E"],
  ["∈", "\\in"],
  ["∉", "\\notin"],
  ["∧", "/\\"],
  ["∨", "\\/"],
  ["¬", "~"],
  ["∪", "\\cup"],
  ["∩", "\\cap"],
  ["⊆", "\\subseteq"],
  ["⊂", "\\subset"],
  ["⊇", "\\supseteq"],
  ["×", "\\X"],
  ["∅", "{}"],
  ["∖", "\\"],
  ["↦", "|->"],
  ["→", "->"],
  ["⊥", "FALSE"],
  ["⊤", "TRUE"],
];

/**
 * Convert TLA+ Unicode glyphs to their ASCII operator forms. Idempotent: text
 * that is already ASCII (e.g. a verbatim predicate parsed from a .tla file) is
 * returned unchanged, because none of the glyph keys appear in it.
 */
export function glyphsToAscii(text: string): string {
  let out = text;
  for (const [glyph, ascii] of GLYPHS) {
    if (out.includes(glyph)) out = out.split(glyph).join(ascii);
  }
  return out;
}

// The heading the sim uses to introduce an inline formal spec inside a doc
// comment, e.g. "TLA+ Specification:  ∀ d ∈ Drivers: …".
const SPEC_HEADING = /TLA\+?\s*Specification:?/i;

/**
 * Convert math cardinality bars to TLA+ `Cardinality(...)`. Doc comments write
 * `|{x \in S : P}|` for "how many"; TLA+ has no `|S|` operator. Only the
 * `{set-builder}` and `(expr)` forms are converted — deliberately NOT bare `|x|`,
 * to avoid mangling record maps (`[a |-> 1, b |-> 2]`) whose `|` chars would
 * otherwise look like a bar pair. Requires `FiniteSets` to be EXTENDed (it is).
 */
export function cardinalityToAscii(text: string): string {
  return text
    .replace(/\|\s*(\{[^{}|]*\})\s*\|/g, "Cardinality($1)")
    .replace(/\|\s*\(([^()|]*)\)\s*\|/g, "Cardinality($1)");
}

/**
 * Whether a predicate is safe to drop into a .tla file as a live operator body.
 *
 * Doc-comment predicates are written for humans and routinely use math notation
 * that is NOT TLA+: a leftover Unicode glyph this module has no ASCII mapping for
 * (e.g. ∑ summation), or `|S|` cardinality bars (TLA+ wants `Cardinality(S)`).
 * Such text parses fine in prose but breaks SANY, so the generator emits it as a
 * "transcribe by hand" comment instead of an active operator. Verbatim predicates
 * parsed from real .tla files are always clean and pass this trivially.
 */
export function asciiTlaClean(text: string): boolean {
  // eslint-disable-next-line no-control-regex
  if (/[^\x00-\x7F]/.test(text)) return false; // any non-ASCII (unmapped glyph)
  if (/\|\s*[{(]|[})]\s*\|/.test(text)) return false; // |{...}| / |(...)| cardinality bars
  return true;
}

/**
 * Pull the formal predicate out of an invariant doc comment, when present.
 *
 * The Go extractor flattens multi-line doc comments into one string with runs
 * of whitespace marking the original paragraph breaks. The predicate follows a
 * "TLA+ Specification:" heading and runs until the next paragraph (a run of two
 * or more spaces), which is where the prose explanation resumes. The result is
 * glyph-converted to ASCII so it can be dropped straight into a .tla operator.
 *
 * Returns undefined when the doc carries no embedded spec.
 */
export function extractDocPredicate(doc: string | undefined): string | undefined {
  if (!doc) return undefined;
  const m = SPEC_HEADING.exec(doc);
  if (!m) return undefined;
  const after = doc.slice(m.index + m[0].length);
  // Paragraphs are separated by 2+ spaces; the predicate is the first non-empty one.
  const para = after
    .split(/\s{2,}/)
    .map((s) => s.trim())
    .find((s) => s.length > 0);
  if (!para) return undefined;
  return cardinalityToAscii(glyphsToAscii(para));
}
