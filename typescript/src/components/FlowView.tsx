// FlowView renders the AST-extracted topology (nodes + edges from model.json)
// with a runtime overlay: node radius and edge width are driven by per-op
// event counts queried from a Parquet trace via DuckDB-WASM.
//
// This file is lazy-loaded from DstAstView so deck.gl, duckdb-wasm, and
// apache-arrow stay out of the core viewer bundle until the flow tab opens.

import { useEffect, useMemo, useRef, useState } from "react";
import DeckGL from "@deck.gl/react";
import { OrthographicView } from "@deck.gl/core";
import { PathLayer, ScatterplotLayer } from "@deck.gl/layers";
import type { PickingInfo } from "@deck.gl/core";

import type { DstModel } from "../types";
import { computeLayout } from "../graphLayout";
import { buildNodeIndex, RADIUS } from "../shared/graphMeta";
import { aggregate, edgeWidth, nodeRadius } from "../flow/aggregate";

export interface FlowViewProps {
  model: DstModel;
  /** Parquet trace URL. When absent the view renders an inert placeholder. */
  tracePath?: string;
  selected: string | null;
  /** One-hop neighbourhood of `selected` (from the shared graph). */
  highlight: Set<string>;
  onSelect: (id: string) => void;
  onClear: () => void;
}

type RGBA = [number, number, number, number];

const CSS_KIND_VAR: Record<string, string> = {
  op: "--dstast-op",
  fault: "--dstast-fault",
  inv: "--dstast-inv",
  truth: "--dstast-truth",
  spec: "--dstast-spec",
};
const CSS_FALLBACK: Record<string, RGBA> = {
  op: [125, 211, 252, 255],
  fault: [252, 165, 165, 255],
  inv: [134, 239, 172, 255],
  truth: [196, 181, 253, 255],
  spec: [251, 191, 36, 255],
};

function hexToRgba(hex: string, alpha = 255): RGBA {
  const clean = hex.trim().replace(/^#/, "");
  if (clean.length !== 6) return [128, 128, 128, alpha];
  const r = parseInt(clean.slice(0, 2), 16);
  const g = parseInt(clean.slice(2, 4), 16);
  const b = parseInt(clean.slice(4, 6), 16);
  return [r, g, b, alpha];
}

// Resolve --dstast-* CSS vars on the host element; fall back to the palette so
// rendering still works if the parent didn't load styles.css.
function readPalette(host: HTMLElement | null): Record<string, RGBA> {
  if (!host) return CSS_FALLBACK;
  const cs = getComputedStyle(host);
  const out: Record<string, RGBA> = {};
  for (const kind of Object.keys(CSS_KIND_VAR)) {
    const raw = cs.getPropertyValue(CSS_KIND_VAR[kind]).trim();
    out[kind] = raw ? hexToRgba(raw) : CSS_FALLBACK[kind];
  }
  return out;
}

export function FlowView({
  model,
  tracePath,
  selected,
  highlight,
  onSelect,
  onClear,
}: FlowViewProps) {
  const hostRef = useRef<HTMLDivElement | null>(null);

  const nodeMeta = useMemo(() => buildNodeIndex(model), [model]);
  const nodeIds = useMemo(() => Array.from(nodeMeta.keys()), [nodeMeta]);
  const layout = useMemo(
    () => computeLayout(nodeIds, model.edges, { width: 1000, height: 700 }),
    [nodeIds, model.edges],
  );

  const [palette, setPalette] = useState<Record<string, RGBA>>(CSS_FALLBACK);
  useEffect(() => {
    setPalette(readPalette(hostRef.current));
  }, []);

  // Stage 1: whole-run per-op counts. Stages 2-5 swap this for windowed +
  // per-bit queries with the same downstream aggregate() shape.
  const [activity, setActivity] = useState<{
    nodes: Map<string, number>;
    edges: { from: string; to: string; count: number }[];
    total: number;
  }>({ nodes: new Map(), edges: [], total: 0 });
  const [status, setStatus] = useState<"idle" | "loading" | "ready" | "error">(
    tracePath ? "loading" : "idle",
  );
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  useEffect(() => {
    if (!tracePath) {
      setStatus("idle");
      return;
    }
    let cancelled = false;
    setStatus("loading");
    setErrorMsg(null);
    (async () => {
      try {
        const { attachTrace } = await import("../flow/duckdb");
        const { queryOpCounts } = await import("../flow/query");
        const conn = await attachTrace(tracePath);
        const { ops, total } = await queryOpCounts(conn);
        if (cancelled) return;
        const agg = aggregate(model, { total, ops });
        setActivity({ nodes: agg.nodeActivity, edges: agg.edgeFlow, total });
        setStatus("ready");
      } catch (err) {
        if (cancelled) return;
        setErrorMsg(err instanceof Error ? err.message : String(err));
        setStatus("error");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [tracePath, model]);

  const focusing = selected !== null;
  const dim = (id: string) => focusing && !highlight.has(id);

  const nodeLayer = useMemo(() => {
    const data = nodeIds
      .map((id) => {
        const p = layout.pos.get(id);
        const meta = nodeMeta.get(id);
        if (!p || !meta) return null;
        const base = RADIUS[meta.kind];
        const count = activity.nodes.get(id) ?? 0;
        const color: RGBA = palette[meta.kind] ?? CSS_FALLBACK[meta.kind];
        return {
          id,
          position: [p.x, layout.height - p.y] as [number, number],
          radius: nodeRadius(base, count),
          color: dim(id) ? ([color[0], color[1], color[2], 60] as RGBA) : color,
          count,
        };
      })
      .filter((x): x is NonNullable<typeof x> => x !== null);
    return new ScatterplotLayer({
      id: "flow-nodes",
      data,
      getPosition: (d) => d.position,
      getRadius: (d) => d.radius,
      getFillColor: (d) => d.color,
      radiusUnits: "pixels",
      lineWidthUnits: "pixels",
      stroked: true,
      getLineColor: [0, 0, 0, 200],
      getLineWidth: 1,
      pickable: true,
      onClick: (info: PickingInfo) => {
        if (info.object && typeof info.object.id === "string") onSelect(info.object.id);
      },
      updateTriggers: {
        getFillColor: [selected, palette],
        getRadius: [activity.nodes],
      },
    });
  }, [nodeIds, nodeMeta, layout, activity.nodes, palette, selected, highlight, onSelect]);

  const edgeLayer = useMemo(() => {
    const data = activity.edges
      .map((e) => {
        const a = layout.pos.get(e.from);
        const b = layout.pos.get(e.to);
        if (!a || !b) return null;
        const lit = !focusing || (highlight.has(e.from) && highlight.has(e.to));
        return {
          path: [
            [a.x, layout.height - a.y] as [number, number],
            [b.x, layout.height - b.y] as [number, number],
          ],
          width: edgeWidth(e.count),
          color: lit ? ([110, 168, 254, 200] as RGBA) : ([110, 168, 254, 40] as RGBA),
        };
      })
      .filter((x): x is NonNullable<typeof x> => x !== null);
    return new PathLayer({
      id: "flow-edges",
      data,
      getPath: (d) => d.path,
      getWidth: (d) => d.width,
      getColor: (d) => d.color,
      widthUnits: "pixels",
      capRounded: true,
      jointRounded: true,
      updateTriggers: {
        getColor: [selected],
        getWidth: [activity.edges],
      },
    });
  }, [activity.edges, layout, focusing, highlight, selected]);

  const onDeckClick = (info: PickingInfo) => {
    if (!info.object && selected) onClear();
  };

  return (
    <div ref={hostRef} className="dstast-flow">
      <DeckGL
        views={new OrthographicView({ id: "flow-ortho", flipY: false })}
        initialViewState={{
          target: [layout.width / 2, layout.height / 2, 0],
          zoom: 0,
        }}
        controller={true}
        layers={[edgeLayer, nodeLayer]}
        onClick={onDeckClick}
      />
      <div className="dstast-flow-legend" role="status">
        {status === "idle" ? (
          <span>
            <b>no trace attached</b> — pass a <code>tracePath</code> prop pointing at a
            Parquet file produced by <code>astdst trace-compact</code> to light the topology
            up with real DST activity.
          </span>
        ) : status === "loading" ? (
          <span>loading <code>{tracePath}</code>…</span>
        ) : status === "error" ? (
          <span>
            <b>trace load failed:</b> {errorMsg}
          </span>
        ) : (
          <span>
            <b>{activity.total.toLocaleString()}</b> events ·{" "}
            node radius = log(events touching) · edge width = log(min endpoint activity)
          </span>
        )}
      </div>
    </div>
  );
}

export default FlowView;
