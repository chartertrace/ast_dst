// Standalone demo of <DstAstView />. Run from ast_dst/typescript:
//
//   npm install && npm run dev      # serves this demo on http://localhost:5173
//
// It shows the same viewer over two models, to make one point: the tool is
// generic. Toggle between
//
//   • "ast_dst itself"  — the extractor pointed at its own golang/ source in the
//     generic `structure` mode (functions / types / packages). Regenerate with:
//        cd ../golang && go run ./cmd/astdst structure --root . \
//          --out ../typescript/sample/self-model.json
//   • "primary-server/sim" — a real DST model (weighted ops, faults, invariants
//     grouped by Truth, TLA+ coverage), the tool's intended use.
//
// Same React component, same node/edge shape; only the model — and the labels it
// carries in `meta` — differ. That is the whole "demo itself" idea: a non-sim Go
// repo (this one) renders honestly through the identical viewer.

import { StrictMode, useState } from "react";
import type { ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { DstAstView } from "../src";
import type { DstModel } from "../src";
import simModel from "../sample/model.json";
import selfModel from "../sample/self-model.json";

type Which = "self" | "sim";

const VIEWS: Record<Which, { model: DstModel; tab: string; blurb: ReactNode }> = {
  self: {
    model: selfModel as DstModel,
    tab: "ast_dst itself",
    blurb: (
      <>
        The extractor pointed at <b>its own Go source</b> in the generic{" "}
        <code>structure</code> mode. The buckets now hold ast_dst&apos;s functions,
        types and packages — same viewer, relabelled by the model&apos;s{" "}
        <code>meta</code>. A non-sim repo rendered honestly: no faults to inject,
        no TLA+ coverage, just structure.
      </>
    ),
  },
  sim: {
    model: simModel as DstModel,
    tab: "primary-server/sim",
    blurb: (
      <>
        A real <b>deterministic simulation test</b> — the tool&apos;s intended use.
        Weighted operations, the faults each injects, and invariants grouped by
        Truth, with their TLA+ spec coverage badged. Compare the shape to the
        self-view on the left: identical component, richer model.
      </>
    ),
  },
};

function Demo() {
  const [which, setWhich] = useState<Which>("self");
  const view = VIEWS[which];

  return (
    <div style={{ position: "fixed", inset: 0, display: "flex", flexDirection: "column" }}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 16,
          padding: "10px 16px",
          background: "#0b0d11",
          borderBottom: "1px solid #232733",
          color: "#c9d1d9",
          font: "13px/1.45 system-ui, -apple-system, sans-serif",
        }}
      >
        <div style={{ display: "flex", gap: 6 }}>
          {(Object.keys(VIEWS) as Which[]).map((key) => {
            const active = key === which;
            return (
              <button
                key={key}
                onClick={() => setWhich(key)}
                style={{
                  padding: "5px 12px",
                  borderRadius: 6,
                  cursor: "pointer",
                  border: active ? "1px solid #3b82f6" : "1px solid #2b3140",
                  background: active ? "#1d2535" : "transparent",
                  color: active ? "#fff" : "#9aa4b2",
                  font: "inherit",
                  fontWeight: active ? 600 : 400,
                }}
              >
                {VIEWS[key].tab}
              </button>
            );
          })}
        </div>
        <p style={{ margin: 0, color: "#8b94a3", maxWidth: 820 }}>{view.blurb}</p>
      </div>

      <div style={{ position: "relative", flex: 1, minHeight: 0 }}>
        {/* key on `which` so selection state resets when the model changes. */}
        <DstAstView key={which} model={view.model} />
      </div>
    </div>
  );
}

const root = document.getElementById("root");
if (root) {
  createRoot(root).render(
    <StrictMode>
      <Demo />
    </StrictMode>,
  );
}
