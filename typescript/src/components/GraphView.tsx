import { useMemo, useRef, useState } from "react";
import type { DstModel, NodeKind } from "../types";
import {
  faultNodeId,
  invariantNodeId,
  nodeKind,
  opNodeId,
  specNodeId,
  truthNodeId,
} from "../types";
import { computeLayout } from "../graphLayout";

export interface GraphViewProps {
  model: DstModel;
  selected: string | null;
  /** One-hop neighbourhood of `selected` (from the shared graph). */
  highlight: Set<string>;
  onSelect: (id: string) => void;
  onClear: () => void;
}

interface NodeMeta {
  label: string;
  title: string;
  kind: NodeKind;
}

// Human labels + kinds for every node id, derived from the typed lists and any
// extra edge endpoints (e.g. spec nodes), so the diagram never invents a node.
function buildIndex(model: DstModel): Map<string, NodeMeta> {
  const meta = new Map<string, NodeMeta>();
  const put = (id: string, label: string, title: string, kind: NodeKind) =>
    meta.set(id, { label, title, kind });

  for (const op of model.operations) put(opNodeId(op.index), op.name, `operation · ${op.name}`, "op");
  for (const f of model.faults) put(faultNodeId(f.id), f.label || f.id, `fault · ${f.label || f.id}`, "fault");
  for (const inv of model.invariants) put(invariantNodeId(inv.id), inv.id, `${inv.id} · ${inv.label}`, "inv");
  for (const t of model.truths) put(truthNodeId(t.name), t.name, `truth · ${t.name}`, "truth");
  for (const s of model.specs ?? []) put(specNodeId(s.name), s.name, `spec · ${s.name}`, "spec");

  // Catch any endpoint without a typed source (defensive; keeps edges drawable).
  for (const e of model.edges) {
    for (const id of [e.from, e.to]) {
      if (meta.has(id)) continue;
      const k = nodeKind(id) ?? "inv";
      put(id, id.slice(id.indexOf(":") + 1), id, k);
    }
  }
  return meta;
}

const RADIUS: Record<NodeKind, number> = { truth: 9, op: 7, fault: 6, inv: 6, spec: 5 };

interface View {
  tx: number;
  ty: number;
  scale: number;
}

/**
 * GraphView draws the extracted model as an interactive node-link diagram: every
 * op/fault/invariant/truth/spec is a node, every extracted edge a line. Layout is
 * deterministic (see computeLayout). Scroll to zoom, drag the background to pan,
 * click a node to focus its one-hop neighbourhood (the rest dims), hover for the
 * full label. Selection is shared with the column view and the detail panel.
 */
export function GraphView({ model, selected, highlight, onSelect, onClear }: GraphViewProps) {
  const index = useMemo(() => buildIndex(model), [model]);
  const nodeIds = useMemo(() => Array.from(index.keys()), [index]);
  const layout = useMemo(
    () => computeLayout(nodeIds, model.edges, { width: 1000, height: 700 }),
    [nodeIds, model.edges],
  );

  const [view, setView] = useState<View>({ tx: 0, ty: 0, scale: 1 });
  const [hovered, setHovered] = useState<string | null>(null);
  const svgRef = useRef<SVGSVGElement | null>(null);
  const drag = useRef<{ x: number; y: number; tx: number; ty: number; moved: boolean } | null>(null);

  const onWheel = (e: React.WheelEvent) => {
    const rect = svgRef.current?.getBoundingClientRect();
    if (!rect) return;
    const sx = e.clientX - rect.left;
    const sy = e.clientY - rect.top;
    const factor = e.deltaY < 0 ? 1.1 : 1 / 1.1;
    const next = Math.min(4, Math.max(0.2, view.scale * factor));
    // Keep the point under the cursor fixed while zooming.
    const px = (sx - view.tx) / view.scale;
    const py = (sy - view.ty) / view.scale;
    setView({ scale: next, tx: sx - px * next, ty: sy - py * next });
  };

  const onPointerDown = (e: React.PointerEvent) => {
    drag.current = { x: e.clientX, y: e.clientY, tx: view.tx, ty: view.ty, moved: false };
    (e.target as Element).setPointerCapture?.(e.pointerId);
  };
  const onPointerMove = (e: React.PointerEvent) => {
    const d = drag.current;
    if (!d) return;
    const dx = e.clientX - d.x;
    const dy = e.clientY - d.y;
    if (Math.abs(dx) + Math.abs(dy) > 3) d.moved = true;
    setView((v) => ({ ...v, tx: d.tx + dx, ty: d.ty + dy }));
  };
  const onPointerUp = () => {
    drag.current = null;
  };
  const onBackgroundClick = () => {
    if (!drag.current?.moved && selected) onClear();
  };

  const focusing = selected !== null;
  const dimNode = (id: string) => focusing && !highlight.has(id);
  // Default to labelling only truths (the few high-level groupers) to avoid a
  // wall of text on dense graphs; everything else labels on hover or focus.
  const showLabel = (id: string, kind: NodeKind) =>
    hovered === id || (focusing ? highlight.has(id) : kind === "truth");

  return (
    <div className="dstast-graph">
      <svg
        ref={svgRef}
        className="dstast-graph-svg"
        role="group"
        aria-label={`Node-link graph: ${nodeIds.length} nodes, ${model.edges.length} edges. The columns view offers the same data with full keyboard navigation.`}
        viewBox={`0 0 ${layout.width} ${layout.height}`}
        preserveAspectRatio="xMidYMid meet"
        onWheel={onWheel}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onClick={onBackgroundClick}
      >
        <g transform={`translate(${view.tx} ${view.ty}) scale(${view.scale})`}>
          {/* edges under nodes */}
          {model.edges.map((e, i) => {
            const a = layout.pos.get(e.from);
            const b = layout.pos.get(e.to);
            if (!a || !b) return null;
            const lit = !focusing || (highlight.has(e.from) && highlight.has(e.to));
            return (
              <line
                key={i}
                className={`dstast-gedge dstast-gedge-${e.kind}`}
                x1={a.x}
                y1={a.y}
                x2={b.x}
                y2={b.y}
                opacity={lit ? undefined : 0.12}
              />
            );
          })}
          {/* nodes + labels */}
          {nodeIds.map((id) => {
            const p = layout.pos.get(id);
            const m = index.get(id);
            if (!p || !m) return null;
            const dimmed = dimNode(id);
            return (
              <g
                key={id}
                className="dstast-gnode-g"
                role="button"
                aria-label={m.title}
                opacity={dimmed ? 0.2 : 1}
                onPointerDown={(ev) => ev.stopPropagation()}
                onClick={(ev) => {
                  ev.stopPropagation();
                  onSelect(id);
                }}
                onMouseEnter={() => setHovered(id)}
                onMouseLeave={() => setHovered((h) => (h === id ? null : h))}
              >
                <title>{m.title}</title>
                <circle
                  className={`dstast-gnode dstast-gnode-${m.kind}${id === selected ? " is-selected" : ""}`}
                  cx={p.x}
                  cy={p.y}
                  r={RADIUS[m.kind]}
                />
                {showLabel(id, m.kind) ? (
                  <text className="dstast-glabel" x={p.x + RADIUS[m.kind] + 3} y={p.y + 3}>
                    {m.label}
                  </text>
                ) : null}
              </g>
            );
          })}
        </g>
      </svg>

      <div className="dstast-graph-legend">
        {(["op", "fault", "inv", "truth", "spec"] as NodeKind[]).map((k) => (
          <span key={k} className="dstast-legend-item">
            <span className={`dstast-legend-dot dstast-gnode-${k}`} />
            {k}
          </span>
        ))}
        <span className="dstast-legend-hint">scroll = zoom · drag = pan · click a node to focus</span>
        {(view.scale !== 1 || view.tx !== 0 || view.ty !== 0) ? (
          <button
            type="button"
            className="dstast-legend-reset"
            onClick={() => setView({ tx: 0, ty: 0, scale: 1 })}
          >
            reset view
          </button>
        ) : null}
      </div>
    </div>
  );
}
