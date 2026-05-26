import type { Fault, Operation } from "../types";
import { faultNodeId, opNodeId } from "../types";
import { rowClass } from "./util";

interface Props {
  operations: Operation[];
  faultByEnum: Map<string, Fault>;
  label: string;
  selected: string | null;
  highlight: Set<string>;
  onSelect: (nodeId: string) => void;
}

/**
 * Left column: the weighted operation table the engine dispatches from. The
 * weight bar is scaled to the heaviest op so relative dispatch frequency reads
 * at a glance; fault chips are the faults the handler body actually injects.
 */
export function OperationList({ operations, faultByEnum, label, selected, highlight, onSelect }: Props) {
  const maxWeight = operations.reduce((m, o) => Math.max(m, o.weight), 0) || 1;
  const dimming = highlight.size > 0;

  return (
    <section className="dstast-col">
      <div className="dstast-col-head">
        <span className="dstast-dot op" />
        {label}
        <span className="dstast-count">{operations.length}</span>
      </div>
      <div className="dstast-col-body">
        {operations.map((op) => {
          const id = opNodeId(op.index);
          const dim = dimming && !highlight.has(id);
          return (
            <button
              key={id}
              type="button"
              className={rowClass(id === selected, dim)}
              aria-pressed={id === selected}
              aria-label={`operation ${op.name}`}
              onClick={() => onSelect(id)}
            >
              <div className="dstast-row-top">
                <span className="dstast-row-name">{op.name}</span>
                <span className="dstast-row-id">#{op.index}</span>
                <div
                  className="dstast-weight"
                  title={`weight ${op.weight} · ${(op.share * 100).toFixed(1)}% of dispatch`}
                >
                  <i style={{ width: `${(op.weight / maxWeight) * 100}%` }} />
                </div>
                <span className="dstast-weight-label">{(op.share * 100).toFixed(0)}%</span>
              </div>
              {op.note ? <div className="dstast-row-note">{op.note}</div> : null}
              <div className="dstast-chips">
                {op.faults.length === 0 ? (
                  <span className="dstast-chip muted">no faults</span>
                ) : (
                  op.faults.map((enumName) => {
                    const f = faultByEnum.get(enumName);
                    const fid = f ? faultNodeId(f.id) : enumName;
                    const chipDim = dimming && !highlight.has(fid);
                    return (
                      <span
                        key={enumName}
                        className="dstast-chip"
                        style={chipDim ? { opacity: 0.4 } : undefined}
                      >
                        {f?.id ?? enumName}
                      </span>
                    );
                  })
                )}
              </div>
            </button>
          );
        })}
      </div>
    </section>
  );
}
