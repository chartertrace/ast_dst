// Minimal standalone demo of <DstAstView />. Run it with any Vite setup that
// has react + react-dom installed, e.g. from the repo's typescript_frontend:
//
//   cd ast_dst/typescript && npm i -D vite @vitejs/plugin-react && npx vite demo
//
// or import DstAstView straight into an existing app (see ../README.md).

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { DstAstView } from "../src";
import type { DstModel } from "../src";
import model from "../sample/model.json";

const root = document.getElementById("root");
if (root) {
  createRoot(root).render(
    <StrictMode>
      <div style={{ position: "fixed", inset: 0 }}>
        <DstAstView model={model as DstModel} />
      </div>
    </StrictMode>,
  );
}
