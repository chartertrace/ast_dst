// Node metadata shared by every view that renders the extracted topology.
// Lifted out of GraphView so the upcoming FlowView (deck.gl) can build the
// same label/title/kind index without duplicating logic — the column view
// only needs raw `model.*` arrays, so it has no use for this module.

import type { DstModel, NodeKind } from "../types";
import {
  faultNodeId,
  invariantNodeId,
  nodeKind,
  opNodeId,
  specNodeId,
  truthNodeId,
} from "../types";

export interface NodeMeta {
  label: string;
  title: string;
  kind: NodeKind;
}

// Pixel radii per node kind; identical between the SVG graph view and the
// deck.gl flow view so the spatial map reads the same in both.
export const RADIUS: Record<NodeKind, number> = {
  truth: 9,
  op: 7,
  fault: 6,
  inv: 6,
  spec: 5,
};

/**
 * buildNodeIndex assigns human labels + kinds to every node id derived from
 * the model's typed lists plus any extra edge endpoints (e.g. spec nodes).
 * Defensive: an edge to an unknown id still gets a stub entry so callers can
 * draw it rather than silently dropping the link.
 */
export function buildNodeIndex(model: DstModel): Map<string, NodeMeta> {
  const meta = new Map<string, NodeMeta>();
  const put = (id: string, label: string, title: string, kind: NodeKind) =>
    meta.set(id, { label, title, kind });

  for (const op of model.operations) put(opNodeId(op.index), op.name, `operation · ${op.name}`, "op");
  for (const f of model.faults) put(faultNodeId(f.id), f.label || f.id, `fault · ${f.label || f.id}`, "fault");
  for (const inv of model.invariants) put(invariantNodeId(inv.id), inv.id, `${inv.id} · ${inv.label}`, "inv");
  for (const t of model.truths) put(truthNodeId(t.name), t.name, `truth · ${t.name}`, "truth");
  for (const s of model.specs ?? []) put(specNodeId(s.name), s.name, `spec · ${s.name}`, "spec");

  for (const e of model.edges) {
    for (const id of [e.from, e.to]) {
      if (meta.has(id)) continue;
      const k = nodeKind(id) ?? "inv";
      put(id, id.slice(id.indexOf(":") + 1), id, k);
    }
  }
  return meta;
}
