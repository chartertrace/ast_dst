import { useEffect, useMemo, useState } from "react";
import type { DstModel } from "./types";
import { buildGraph, highlightSet } from "./graph";
import { OperationList } from "./components/OperationList";
import { FaultList } from "./components/FaultList";
import { InvariantTree } from "./components/InvariantTree";
import { DetailPanel } from "./components/DetailPanel";
import { GraphView } from "./components/GraphView";
import "./styles.css";

type ViewMode = "columns" | "graph";

export interface DstAstViewProps {
  /** The model emitted by the `astdst` Go extractor (model.json). */
  model: DstModel;
  /** Optional extra class on the root, e.g. to override CSS variables. */
  className?: string;
}

/**
 * DstAstView renders the static structure of the deterministic simulation test
 * — operations, the faults they inject, and the invariants those exercise,
 * grouped by Truth — from the AST-extracted model. Selecting any node
 * highlights its one-hop neighbourhood and shows its source doc + location.
 *
 * It is fully self-contained: pass the parsed model.json and drop it anywhere.
 */
export function DstAstView({ model, className }: DstAstViewProps) {
  const graph = useMemo(() => buildGraph(model), [model]);
  const [selected, setSelected] = useState<string | null>(null);
  const [view, setView] = useState<ViewMode>("columns");
  const highlight = useMemo(() => highlightSet(graph, selected), [graph, selected]);

  const onSelect = (nodeId: string) =>
    setSelected((cur) => (cur === nodeId ? null : nodeId));

  // Escape clears the current selection (and its highlight), wherever focus is.
  useEffect(() => {
    if (!selected) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setSelected(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [selected]);

  // Display labels: the DST defaults, or the model's own (e.g. `structure` mode).
  const L = model.meta?.labels;
  const title = model.meta?.title ?? "DST · static structure";

  return (
    <div className={className ? `dstast ${className}` : "dstast"}>
      <header className="dstast-header">
        <div>
          <div className="dstast-title">{title}</div>
          <div className="dstast-sub">
            parsed from <code>{model.source.simPath}</code> · {model.source.filesRead} files ·{" "}
            {new Date(model.generatedAt).toLocaleString()}
          </div>
        </div>
        <div className="dstast-stats">
          <span className="dstast-stat">
            <b>{model.stats.operations}</b>
            <span>{L ? L.operations : "ops"}</span>
          </span>
          <span className="dstast-stat">
            <b>{model.stats.faults}</b>
            <span>{L ? L.faults : "faults"}</span>
          </span>
          <span className="dstast-stat">
            <b>{model.stats.invariants}</b>
            <span>{L ? L.invariants : "invariants"}</span>
          </span>
          <span className="dstast-stat">
            <b>{model.stats.truths}</b>
            <span>{L ? L.truths : "truths"}</span>
          </span>
          <span className="dstast-stat">
            <b>{model.stats.edges}</b>
            <span>edges</span>
          </span>
          {model.stats.specsTotal > 0 ? (
            <span
              className="dstast-stat"
              title={`${model.stats.validated} model-checked · ${model.stats.uncheckedSpec} specified-only · ${model.stats.unspecified} unspecified`}
            >
              <b>
                {model.stats.invariants - model.stats.unspecified}/{model.stats.invariants}
              </b>
              <span>specified · {model.stats.validated} checked</span>
            </span>
          ) : null}
        </div>
        <div className="dstast-viewtoggle" role="group" aria-label="view mode">
          {(["columns", "graph"] as ViewMode[]).map((m) => (
            <button
              key={m}
              type="button"
              aria-pressed={view === m}
              aria-label={`${m} view`}
              className={`dstast-viewtoggle-btn${view === m ? " is-active" : ""}`}
              onClick={() => setView(m)}
            >
              {m}
            </button>
          ))}
        </div>
      </header>

      {view === "columns" ? (
        <div className="dstast-columns" role="group" aria-label="operations, faults and invariants">

          <OperationList
            operations={model.operations}
            faultByEnum={graph.faultByEnum}
            label={L?.operations ?? "Operations"}
            selected={selected}
            highlight={highlight}
            onSelect={onSelect}
          />
          <FaultList
            faults={model.faults}
            opsByFaultId={graph.opsByFaultId}
            label={L?.faults ?? "Faults"}
            selected={selected}
            highlight={highlight}
            onSelect={onSelect}
          />
          <InvariantTree
            truths={model.truths}
            invariants={model.invariants}
            label={L?.invariants ?? "Invariants"}
            groupLabel={L?.truths ?? "truths"}
            selected={selected}
            highlight={highlight}
            onSelect={onSelect}
          />
        </div>
      ) : (
        <GraphView
          model={model}
          selected={selected}
          highlight={highlight}
          onSelect={onSelect}
          onClear={() => setSelected(null)}
        />
      )}

      <DetailPanel model={model} selected={selected} onClear={() => setSelected(null)} />
    </div>
  );
}
