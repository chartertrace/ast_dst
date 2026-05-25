import { useMemo, useState } from "react";
import type { DstModel } from "./types";
import { buildGraph, highlightSet } from "./graph";
import { OperationList } from "./components/OperationList";
import { FaultList } from "./components/FaultList";
import { InvariantTree } from "./components/InvariantTree";
import { DetailPanel } from "./components/DetailPanel";
import "./styles.css";

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
  const highlight = useMemo(() => highlightSet(graph, selected), [graph, selected]);

  const onSelect = (nodeId: string) =>
    setSelected((cur) => (cur === nodeId ? null : nodeId));

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
      </header>

      <div className="dstast-columns">
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

      <DetailPanel model={model} selected={selected} onClear={() => setSelected(null)} />
    </div>
  );
}
