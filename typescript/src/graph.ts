// Pure graph helpers over the extracted model. The viewer highlights the
// one-hop neighbourhood of whatever node you select; all of that is derived
// from the edge list the Go extractor emits, so the UI never invents links.

import type { DstModel, Edge, Fault, Operation } from "./types";

export interface Graph {
  /** Undirected one-hop adjacency, keyed by namespaced node id. */
  neighbors: Map<string, Set<string>>;
  /** Edges touching a given node id (for drawing/explaining a selection). */
  edgesOf: Map<string, Edge[]>;
  /** Fault enum name ("FaultCreditRace") -> fault, for op chip rendering. */
  faultByEnum: Map<string, Fault>;
  /** Fault id ("credit_race") -> the ops that inject it. */
  opsByFaultId: Map<string, Operation[]>;
}

export function buildGraph(model: DstModel): Graph {
  const neighbors = new Map<string, Set<string>>();
  const edgesOf = new Map<string, Edge[]>();

  const link = (a: string, b: string) => {
    if (!neighbors.has(a)) neighbors.set(a, new Set());
    if (!neighbors.has(b)) neighbors.set(b, new Set());
    neighbors.get(a)!.add(b);
    neighbors.get(b)!.add(a);
  };
  const touch = (id: string, e: Edge) => {
    if (!edgesOf.has(id)) edgesOf.set(id, []);
    edgesOf.get(id)!.push(e);
  };

  for (const e of model.edges) {
    link(e.from, e.to);
    touch(e.from, e);
    touch(e.to, e);
  }

  const faultByEnum = new Map<string, Fault>();
  for (const f of model.faults) faultByEnum.set(f.enum, f);

  const faultIdByEnum = new Map<string, string>();
  for (const f of model.faults) faultIdByEnum.set(f.enum, f.id);

  const opsByFaultId = new Map<string, Operation[]>();
  for (const op of model.operations) {
    // `structure` mode emits faults: null; guard so the graph still builds.
    for (const enumName of op.faults ?? []) {
      const id = faultIdByEnum.get(enumName);
      if (!id) continue;
      if (!opsByFaultId.has(id)) opsByFaultId.set(id, []);
      opsByFaultId.get(id)!.push(op);
    }
  }

  return { neighbors, edgesOf, faultByEnum, opsByFaultId };
}

/**
 * Returns the set of node ids to keep highlighted for a selection: the node
 * itself plus its direct neighbours. Empty selection => empty set (caller
 * treats "nothing selected" as "everything bright").
 */
export function highlightSet(graph: Graph, selected: string | null): Set<string> {
  const out = new Set<string>();
  if (!selected) return out;
  out.add(selected);
  for (const n of graph.neighbors.get(selected) ?? []) out.add(n);
  return out;
}
