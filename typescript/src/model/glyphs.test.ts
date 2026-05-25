import { describe, expect, it } from "vitest";
import { cardinalityToAscii, extractDocPredicate, glyphsToAscii } from "./glyphs";

describe("glyphsToAscii", () => {
  it("converts the common TLA+ glyphs", () => {
    expect(glyphsToAscii("∀ d ∈ S: x ≤ 1 ∧ y ⇒ z")).toBe("\\A d \\in S: x <= 1 /\\ y => z");
    expect(glyphsToAscii("∃ e ∈ T: a ≥ b ∨ ¬c")).toBe("\\E e \\in T: a >= b \\/ ~c");
    expect(glyphsToAscii("a ≠ b")).toBe("a # b");
  });

  it("is idempotent on already-ASCII text", () => {
    const ascii = "\\A d \\in Drivers: bal[d] >= 0 /\\ TRUE";
    expect(glyphsToAscii(ascii)).toBe(ascii);
  });
});

describe("cardinalityToAscii", () => {
  it("converts |{set-builder}| to Cardinality(...)", () => {
    expect(cardinalityToAscii("|{s \\in S : P(s)}| <= 1")).toBe("Cardinality({s \\in S : P(s)}) <= 1");
  });

  it("leaves record maps with |-> untouched", () => {
    const rec = "[a |-> 1, b |-> 2]";
    expect(cardinalityToAscii(rec)).toBe(rec);
  });
});

describe("extractDocPredicate", () => {
  it("pulls the predicate after the TLA+ Specification heading, ASCII-converted", () => {
    const doc =
      "CheckIdentity4 verifies IDENTITY-4: SingleActiveShipmentPerDriver. " +
      "TLA+ Specification:  ∀ d ∈ Drivers: x ≤ 1  This invariant prevents: things";
    expect(extractDocPredicate(doc)).toBe("\\A d \\in Drivers: x <= 1");
  });

  it("returns undefined when there is no embedded spec", () => {
    expect(extractDocPredicate("Just a plain doc comment.")).toBeUndefined();
    expect(extractDocPredicate(undefined)).toBeUndefined();
  });
});
