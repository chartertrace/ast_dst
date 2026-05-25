import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Two jobs:
//  - `vite build`  bundles the interactive viewer island (src/site/viewer-entry)
//    into dist-viewer/viewer.js + viewer.css, which gen:docs copies into the
//    generated site's assets/.
//  - `vite demo`   serves demo/ for local development of <DstAstView/>.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist-viewer",
    emptyOutDir: true,
    cssCodeSplit: false,
    rollupOptions: {
      input: "src/site/viewer-entry.tsx",
      output: {
        format: "es",
        entryFileNames: "viewer.js",
        assetFileNames: "viewer.[ext]",
      },
    },
  },
});
