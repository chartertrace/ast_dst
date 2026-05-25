import type { DstModel, Loc, SpecStatus } from "../types";
import { nodeKind } from "../types";
import { genBadge, specBadge } from "./util";

interface Props {
  model: DstModel;
  selected: string | null;
  onClear: () => void;
}

// The TLA+ spec side of an invariant, shown next to its Go checker.
interface SpecView {
  status: SpecStatus;
  name?: string;
  predicate?: string;
  loc?: Loc;
  generated?: boolean;
  verified?: boolean;
  tlcStates?: number;
  tlcDepth?: number;
}

interface Detail {
  kind: string;
  dotClass: string;
  title: string;
  subtitle?: string;
  doc?: string;
  loc?: Loc;
  spec?: SpecView;
}

/**
 * Bottom panel: the full text the columns can't fit — the selected node's
 * doc comment and the source line it was extracted from. With nothing
 * selected it explains the interaction instead of showing a blank box.
 */
export function DetailPanel({ model, selected, onClear }: Props) {
  const detail = selected ? resolve(model, selected) : null;

  if (!detail) {
    return (
      <div className="dstast-detail">
        <div className="dstast-empty">
          Select an operation, fault, truth, or invariant to see its doc comment
          and the source line it was parsed from. Selecting highlights the nodes
          it links to.
        </div>
      </div>
    );
  }

  return (
    <div className="dstast-detail">
      <div className="dstast-detail-head">
        <span className={`dstast-dot ${detail.dotClass}`} />
        <span className="dstast-detail-kind">{detail.kind}</span>
        <span className="dstast-detail-title">{detail.title}</span>
        <button type="button" className="dstast-clear" onClick={onClear}>
          clear
        </button>
      </div>
      {detail.doc ? (
        <div className="dstast-detail-doc">{detail.doc}</div>
      ) : (
        <div className="dstast-empty">No doc comment in source.</div>
      )}
      <div className="dstast-detail-meta">
        {detail.subtitle ? <span>{detail.subtitle}</span> : null}
        {detail.loc ? (
          <span className="dstast-detail-loc">
            {detail.loc.file}:{detail.loc.line}
          </span>
        ) : null}
      </div>
      {detail.spec ? <SpecBlock spec={detail.spec} /> : null}
    </div>
  );
}

/**
 * The formal-spec half of an invariant: its TLA+ predicate and source line,
 * placed beside the runtime checker above. When the invariant has no spec, it
 * says so plainly — the sim validates more than the spec formalises.
 */
function SpecBlock({ spec }: { spec: SpecView }) {
  const badge = specBadge(spec.status);
  const gen = genBadge(spec.generated ? { name: spec.name ?? "", specFile: "", checked: false, generated: spec.generated, verified: spec.verified } : undefined);
  return (
    <div className="dstast-spec-block">
      <div className="dstast-spec-head">
        {badge ? <span className={`dstast-spec-badge ${badge.cls}`}>{badge.glyph}</span> : null}
        <span className="dstast-spec-status">{badge?.title}</span>
        {spec.name ? <span className="dstast-spec-name">TLA+ {spec.name}</span> : null}
        {spec.loc ? (
          <span className="dstast-detail-loc">
            {spec.loc.file}:{spec.loc.line}
          </span>
        ) : null}
      </div>
      {gen ? (
        <div className="dstast-spec-prov">
          <span className={`dstast-spec-badge ${gen.cls}`}>{gen.glyph}</span>
          <span>{gen.title}</span>
          {spec.verified && spec.tlcStates ? (
            <span className="dstast-detail-loc">
              {spec.tlcStates} states · depth {spec.tlcDepth ?? 0}
            </span>
          ) : null}
        </div>
      ) : null}
      {spec.predicate ? (
        <pre className="dstast-spec-pred">{spec.predicate}</pre>
      ) : spec.status === "unspecified" ? (
        <div className="dstast-empty">No TLA+ predicate — this invariant is enforced only by the sim.</div>
      ) : null}
      {spec.predicate || gen ? (
        <div className="dstast-empty">
          TLC checks the TLA+ model; it does not prove the Go code refines it.
        </div>
      ) : null}
    </div>
  );
}

function resolve(model: DstModel, nodeId: string): Detail | null {
  const kind = nodeKind(nodeId);
  const key = nodeId.slice(nodeId.indexOf(":") + 1);
  const L = model.meta?.labels; // non-DST mode relabels the node kinds

  switch (kind) {
    case "op": {
      const op = model.operations.find((o) => String(o.index) === key);
      if (!op) return null;
      return {
        kind: L ? L.operations.toLowerCase() : "operation",
        dotClass: "op",
        title: op.name,
        subtitle: `weight ${op.weight} · ${(op.share * 100).toFixed(1)}% of dispatch · injects ${op.faults.length} fault${op.faults.length === 1 ? "" : "s"}`,
        doc: op.note,
        loc: op.loc,
      };
    }
    case "fault": {
      const f = model.faults.find((x) => x.id === key);
      if (!f) return null;
      return { kind: L ? L.faults.toLowerCase() : "fault", dotClass: "fault", title: f.label, subtitle: f.id, doc: f.doc, loc: f.loc };
    }
    case "inv": {
      const inv = model.invariants.find((x) => x.id === key);
      if (!inv) return null;
      let spec: SpecView | undefined;
      if (inv.specStatus) {
        const full = inv.spec ? model.specs?.find((s) => s.name === inv.spec!.name) : undefined;
        spec = {
          status: inv.specStatus,
          name: inv.spec?.name,
          predicate: full?.predicate,
          loc: inv.spec?.loc,
          generated: full?.generated ?? inv.spec?.generated,
          verified: full?.verified ?? inv.spec?.verified,
          tlcStates: full?.tlcStates,
          tlcDepth: full?.tlcDepth,
        };
      }
      const invKind = L ? `${inv.truth} · ${L.invariants.toLowerCase()}` : `${inv.truth} invariant`;
      return { kind: invKind, dotClass: "inv", title: inv.label, subtitle: inv.id, doc: inv.doc, loc: inv.loc, spec };
    }
    case "truth": {
      const t = model.truths.find((x) => x.name === key);
      if (!t) return null;
      const labels = model.invariants
        .filter((i) => i.truth === t.name)
        .map((i) => i.label)
        .join(", ");
      return {
        kind: L ? L.truths.toLowerCase() : "truth",
        dotClass: "truth",
        title: t.name,
        subtitle: `${t.count} ${L ? L.invariants.toLowerCase() : "invariants"}`,
        doc: labels,
      };
    }
    default:
      return null;
  }
}
