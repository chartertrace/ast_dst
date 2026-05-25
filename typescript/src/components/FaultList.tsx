import type { Fault, Operation } from "../types";
import { faultNodeId } from "../types";
import { rowClass } from "./util";

interface Props {
  faults: Fault[];
  opsByFaultId: Map<string, Operation[]>;
  selected: string | null;
  highlight: Set<string>;
  onSelect: (nodeId: string) => void;
}

/**
 * Middle column: the fault catalogue. Each fault shows how many operations
 * inject it — a fault no op injects is dead weight, and reads as "0 ops".
 */
export function FaultList({ faults, opsByFaultId, selected, highlight, onSelect }: Props) {
  const dimming = highlight.size > 0;

  return (
    <section className="dstast-col">
      <div className="dstast-col-head">
        <span className="dstast-dot fault" />
        Faults
        <span className="dstast-count">{faults.length}</span>
      </div>
      <div className="dstast-col-body">
        {faults.map((f) => {
          const id = faultNodeId(f.id);
          const dim = dimming && !highlight.has(id);
          const injectors = opsByFaultId.get(f.id)?.length ?? 0;
          return (
            <button
              key={id}
              type="button"
              className={rowClass(id === selected, dim)}
              onClick={() => onSelect(id)}
            >
              <div className="dstast-row-top">
                <span className="dstast-row-name">{f.label}</span>
                <span className="dstast-row-id" style={{ marginLeft: "auto" }}>
                  {injectors} op{injectors === 1 ? "" : "s"}
                </span>
              </div>
              <div className="dstast-row-note">{f.id}</div>
            </button>
          );
        })}
      </div>
    </section>
  );
}
