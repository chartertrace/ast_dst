import { useState } from "react";
import type { Invariant, Truth } from "../types";
import { invariantNodeId, truthNodeId } from "../types";
import { genBadge, rowClass, specBadge } from "./util";

interface Props {
  truths: Truth[];
  invariants: Invariant[];
  label: string;
  groupLabel: string;
  selected: string | null;
  highlight: Set<string>;
  onSelect: (nodeId: string) => void;
}

/**
 * Right column: the 50 invariants grouped under their nine Truths, each group
 * collapsible. Selecting a Truth highlights its whole group; selecting an
 * invariant drills into one property (doc + source line in the detail panel).
 */
export function InvariantTree({ truths, invariants, label, groupLabel, selected, highlight, onSelect }: Props) {
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const dimming = highlight.size > 0;

  const byTruth = new Map<string, Invariant[]>();
  for (const inv of invariants) {
    if (!byTruth.has(inv.truth)) byTruth.set(inv.truth, []);
    byTruth.get(inv.truth)!.push(inv);
  }

  const toggle = (name: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev);
      next.has(name) ? next.delete(name) : next.add(name);
      return next;
    });

  return (
    <section className="dstast-col">
      <div className="dstast-col-head">
        <span className="dstast-dot inv" />
        {label}
        <span className="dstast-count">
          {invariants.length} across {truths.length} {groupLabel}
        </span>
      </div>
      <div className="dstast-col-body">
        {truths.map((truth) => {
          const tid = truthNodeId(truth.name);
          const open = !collapsed.has(truth.name);
          const groupDim = dimming && !highlight.has(tid);
          return (
            <div className="dstast-truth-group" key={tid}>
              <div
                className={`dstast-truth-head${tid === selected ? " selected" : ""}`}
                style={groupDim ? { opacity: 0.4 } : undefined}
              >
                <span
                  className={`dstast-caret${open ? " open" : ""}`}
                  onClick={(e) => {
                    e.stopPropagation();
                    toggle(truth.name);
                  }}
                  role="button"
                  aria-label={open ? "collapse" : "expand"}
                >
                  ▶
                </span>
                <span className="dstast-dot truth" />
                <span
                  className="dstast-row-name"
                  style={{ cursor: "pointer" }}
                  onClick={() => onSelect(tid)}
                >
                  {truth.name}
                </span>
                <span className="dstast-count">{truth.count}</span>
              </div>
              {open ? (
                <div className="dstast-truth-items">
                  {(byTruth.get(truth.name) ?? []).map((inv) => {
                    const id = invariantNodeId(inv.id);
                    const dim = dimming && !highlight.has(id);
                    const badge = specBadge(inv.specStatus);
                    const gen = genBadge(inv.spec);
                    return (
                      <button
                        key={id}
                        type="button"
                        className={rowClass(id === selected, dim)}
                        onClick={() => onSelect(id)}
                      >
                        <div className="dstast-row-top">
                          {badge ? (
                            <span
                              className={`dstast-spec-badge ${badge.cls}`}
                              title={badge.title}
                            >
                              {badge.glyph}
                            </span>
                          ) : null}
                          <span className="dstast-row-name">{inv.label}</span>
                          {gen ? (
                            <span
                              className={`dstast-spec-badge ${gen.cls}`}
                              title={gen.title}
                              style={{ marginLeft: "auto" }}
                            >
                              {gen.glyph}
                            </span>
                          ) : null}
                          <span
                            className="dstast-row-id"
                            style={gen ? undefined : { marginLeft: "auto" }}
                          >
                            {inv.id}
                          </span>
                        </div>
                      </button>
                    );
                  })}
                </div>
              ) : null}
            </div>
          );
        })}
      </div>
    </section>
  );
}
