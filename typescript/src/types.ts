// Types mirroring the JSON emitted by the `astdst` Go extractor
// (ast_dst/golang). Keep in sync with extract/model.go — the Go side is the
// source of truth; this is the wire contract the viewer consumes.

export interface Loc {
  file: string;
  line: number;
}

export interface Truth {
  name: string;
  count: number;
}

// Display labels for an alternate extraction mode. Absent for the default DST
// model (the viewer then uses its built-in DST labels); set by `structure` mode
// so the same viewer reads "Functions / Types / Packages".
export interface Labels {
  operations: string; // column 1 head
  faults: string; // column 2 head
  invariants: string; // column 3 items
  truths: string; // column 3 groups
}

export interface Meta {
  mode: string; // "structure"
  title: string; // header title
  labels: Labels;
}

// Spec-coverage classification of a runtime invariant checker against the
// formal TLA+ spec. Mirrors the Go SpecStatus constants.
export type SpecStatus = "validated" | "unchecked-spec" | "unspecified";

// Back-reference from a Go invariant to the TLA+ invariant it validates.
export interface SpecRef {
  name: string;
  specFile: string;
  checked: boolean;
  loc?: Loc;
  // Provenance: set when the spec was machine-generated and TLC-verified
  // (see the Go `gen` package). Absent for hand-written specs. `behavioral`
  // distinguishes a meaningful check from a merely well-formed one.
  generated?: boolean;
  verified?: boolean;
  behavioral?: boolean;
}

// One invariant declared in a TLA+ specification.
export interface SpecInvariant {
  name: string;
  predicate: string;
  specFile: string;
  checked: boolean;
  doc?: string;
  loc?: Loc;
  // Provenance for machine-generated specs. `verified` is true only when TLC
  // model-checked the spec clean; `tlcStates`/`tlcDepth` describe that search.
  generated?: boolean;
  verified?: boolean;
  behavioral?: boolean;
  tlcStates?: number;
  tlcDepth?: number;
}

// One component of the mutable state model (TLA+ VARIABLES). Present only when a
// state type is configured; the structural viewer ignores it.
export interface StateVar {
  name: string;
  type: string;
  doc?: string;
  loc?: Loc;
}

export interface StateModel {
  typeName: string;
  kind: "struct" | "interface";
  variables: StateVar[];
  loc?: Loc;
}

export interface Invariant {
  id: string;
  label: string;
  truth: string;
  doc?: string;
  loc?: Loc;
  // Present only when a TLA+ spec is configured for the extraction.
  spec?: SpecRef;
  specStatus?: SpecStatus;
}

export interface Fault {
  id: string;
  enum: string;
  label: string;
  doc?: string;
  loc?: Loc;
}

export interface Operation {
  index: number;
  handler: string;
  name: string;
  weight: number;
  share: number;
  note?: string;
  faults: string[]; // fault enum names, e.g. "FaultCreditRace"
  loc?: Loc;
}

export type EdgeKind =
  | "truth-has-invariant"
  | "op-injects-fault"
  | "mentions-invariant"
  | "validates";

export interface Edge {
  from: string; // namespaced node id, e.g. "op:3"
  to: string;
  kind: EdgeKind;
}

export interface Source {
  simPath: string;
  filesRead: number;
  toolModule: string;
}

export interface Stats {
  truths: number;
  invariants: number;
  faults: number;
  operations: number;
  totalWeight: number;
  edges: number;
  // Spec-coverage totals (zero when no TLA+ spec is configured).
  specsTotal: number;
  validated: number;
  uncheckedSpec: number;
  unspecified: number;
}

export interface DstModel {
  generatedAt: string;
  source: Source;
  truths: Truth[];
  invariants: Invariant[];
  faults: Fault[];
  operations: Operation[];
  // Present only when a state type is configured (used by the TLA+ generator).
  state?: StateModel;
  specs?: SpecInvariant[];
  edges: Edge[];
  stats: Stats;
  // Present only in non-DST modes (e.g. `structure`); drives the viewer's labels.
  meta?: Meta;
}

// Node id helpers — the viewer keys selection and edges by these strings, which
// must match the namespacing the Go extractor uses in buildEdges.
export const truthNodeId = (name: string): string => `truth:${name}`;
export const invariantNodeId = (id: string): string => `inv:${id}`;
export const faultNodeId = (id: string): string => `fault:${id}`;
export const opNodeId = (index: number): string => `op:${index}`;
export const specNodeId = (name: string): string => `spec:${name}`;

export type NodeKind = "truth" | "inv" | "fault" | "op" | "spec";

export const nodeKind = (nodeId: string): NodeKind | undefined => {
  const prefix = nodeId.split(":", 1)[0];
  if (
    prefix === "truth" ||
    prefix === "inv" ||
    prefix === "fault" ||
    prefix === "op" ||
    prefix === "spec"
  ) {
    return prefix;
  }
  return undefined;
};
