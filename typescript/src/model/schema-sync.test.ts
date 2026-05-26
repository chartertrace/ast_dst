// Ties the Go wire contract to the TypeScript types: every field the Go
// extractor emits (a `json:` tag on a struct in golang/extract/model.go) must be
// declared on the corresponding TS interface in src/types.ts. The Go side is the
// source of truth (it produces the JSON); this test fails CI when a Go struct
// gains a field that types.ts hasn't mirrored — the exact drift that previously
// left Operation.writes/effects untyped on the viewer side.
//
// It is a *coverage* check, not codegen: it preserves the hand-written TS
// refinements (union types, node-id helpers) that a generator would clobber.
import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const modelGo = readFileSync(resolve(here, "../../../golang/extract/model.go"), "utf8");
const typesTs = readFileSync(resolve(here, "../types.ts"), "utf8");

// Go struct name -> TS interface name. Identity except the top-level rename.
const RENAME: Record<string, string> = { Model: "DstModel" };

// goStructFields maps each `type X struct { ... }` to the json field names it
// emits (the part of the tag before any ",omitempty"; "-" fields are skipped).
function goStructFields(src: string): Map<string, string[]> {
  const out = new Map<string, string[]>();
  const struct = /type\s+(\w+)\s+struct\s*\{([\s\S]*?)\n\}/g;
  for (let m = struct.exec(src); m; m = struct.exec(src)) {
    const fields: string[] = [];
    const tag = /json:"([^"]*)"/g;
    for (let t = tag.exec(m[2]); t; t = tag.exec(m[2])) {
      const field = t[1].split(",")[0];
      if (field && field !== "-") fields.push(field);
    }
    out.set(m[1], fields);
  }
  return out;
}

// tsInterfaceProps maps each `export interface X { ... }` to its property names.
function tsInterfaceProps(src: string): Map<string, Set<string>> {
  const out = new Map<string, Set<string>>();
  const iface = /export\s+interface\s+(\w+)\s*\{([\s\S]*?)\n\}/g;
  for (let m = iface.exec(src); m; m = iface.exec(src)) {
    const props = new Set<string>();
    for (const line of m[2].split("\n")) {
      const p = line.match(/^\s*([A-Za-z_]\w*)\??\s*:/);
      if (p) props.add(p[1]);
    }
    out.set(m[1], props);
  }
  return out;
}

const goStructs = goStructFields(modelGo);
const tsIfaces = tsInterfaceProps(typesTs);

describe("Go model.go ↔ TS types.ts schema agreement", () => {
  it("parses at least the known structs (guards the regexes themselves)", () => {
    expect(goStructs.has("Operation")).toBe(true);
    expect(tsIfaces.has("DstModel")).toBe(true);
  });

  for (const [goName, fields] of goStructs) {
    const tsName = RENAME[goName] ?? goName;
    it(`${goName} → ${tsName} declares every emitted field`, () => {
      const props = tsIfaces.get(tsName);
      expect(props, `no TS interface "${tsName}" for Go struct "${goName}"`).toBeDefined();
      const missing = fields.filter((f) => !props!.has(f));
      expect(missing, `${tsName} is missing field(s) the Go extractor emits`).toEqual([]);
    });
  }
});

// goConstValues returns the string values of `Name = "value"` consts whose name
// starts with prefix — the Go side of an enum-like wire contract.
function goConstValues(src: string, prefix: string): Set<string> {
  const out = new Set<string>();
  const re = new RegExp(`\\b${prefix}\\w+\\s*=\\s*"([^"]+)"`, "g");
  for (let m = re.exec(src); m; m = re.exec(src)) out.add(m[1]);
  return out;
}

// tsUnionMembers returns the string-literal members of `export type Name = ...;`.
function tsUnionMembers(src: string, name: string): Set<string> {
  const out = new Set<string>();
  const block = src.match(new RegExp(`export type ${name}\\s*=([^;]*);`));
  if (block) {
    const lit = /"([^"]+)"/g;
    for (let m = lit.exec(block[1]); m; m = lit.exec(block[1])) out.add(m[1]);
  }
  return out;
}

// The struct fields are tied above; these are the enum-like *string* contracts
// the viewer switches on (edge.kind, invariant.specStatus). The Go consts and
// the TS union literals must name the same set, or the viewer mishandles a value
// the extractor emits.
describe("Go enum constants ↔ TS union literals", () => {
  const cases = [
    { go: "Edge", ts: "EdgeKind" },
    { go: "Spec", ts: "SpecStatus" },
  ];
  for (const { go, ts } of cases) {
    it(`${go}* consts == TS ${ts} union`, () => {
      const goVals = goConstValues(modelGo, go);
      const tsVals = tsUnionMembers(typesTs, ts);
      expect(goVals.size, `no Go ${go}* string consts found`).toBeGreaterThan(0);
      expect(tsVals.size, `no TS ${ts} union found`).toBeGreaterThan(0);
      expect(
        [...goVals].filter((v) => !tsVals.has(v)),
        `TS ${ts} is missing value(s) the Go ${go}* consts define`,
      ).toEqual([]);
      expect(
        [...tsVals].filter((v) => !goVals.has(v)),
        `TS ${ts} has value(s) with no matching Go ${go}* const`,
      ).toEqual([]);
    });
  }
});
