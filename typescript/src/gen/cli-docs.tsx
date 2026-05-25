// CLI: render a static, godoc-style documentation site from model.json.
//
//   tsx src/gen/cli-docs.tsx --in sample/model.json --out site --repo-base <url>
//
// Flags:
//   --in <file>        model.json to read         (default: sample/model.json)
//   --out <dir>        output directory           (default: site)
//   --repo-base <url>  base URL for source links  (optional; e.g. a GitHub blob URL)
//   --assets <dir>     built viewer island dir    (default: dist-viewer)
//
// Each page is rendered with ReactDOMServer to self-contained HTML (doc styles
// are inlined). The overview embeds the interactive <DstAstView/> as a hydrated
// island, loaded from assets/ built separately by Vite.

import {
  cpSync,
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  writeFileSync,
} from "node:fs";
import { dirname, join, resolve } from "node:path";
import type { ReactElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import type { DstModel } from "../types";
import { parseArgs } from "./args";
import * as P from "../site/pages";
import * as L from "../site/links";

interface Page {
  path: string; // root-relative output path, e.g. "invariants/PAYMENT-11.html"
  element: ReactElement;
}

function html(element: ReactElement): string {
  return "<!doctype html>\n" + renderToStaticMarkup(element);
}

/** JSON safe to inline inside a <script> block. */
function inlineJson(model: DstModel): string {
  return JSON.stringify(model).replace(/</g, "\\u003c");
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  const inPath = resolve(args.in ?? "sample/model.json");
  const outDir = resolve(args.out ?? "site");
  const assetsDir = resolve(args.assets ?? "dist-viewer");
  const repoBase = args["repo-base"];

  const model = JSON.parse(readFileSync(inPath, "utf8")) as DstModel;

  // Viewer island is optional: if its build output is present, copy it in and
  // wire the overview to it; otherwise the overview shows a fallback note.
  const hasViewer = existsSync(join(assetsDir, "viewer.js"));
  if (hasViewer) {
    cpSync(assetsDir, join(outDir, "assets"), { recursive: true });
  }
  const hasViewerCss = hasViewer && existsSync(join(assetsDir, "viewer.css"));

  const ctx = (path: string): P.Ctx => ({ model, path, repoBase });

  const overviewHead = hasViewerCss ? (
    <link rel="stylesheet" href="assets/viewer.css" />
  ) : undefined;
  const overviewEnd = hasViewer ? (
    <>
      <script
        id="dst-model"
        type="application/json"
        dangerouslySetInnerHTML={{ __html: inlineJson(model) }}
      />
      <script type="module" src="assets/viewer.js" />
    </>
  ) : undefined;

  const pages: Page[] = [];

  pages.push({
    path: L.OVERVIEW,
    element: (
      <P.Layout ctx={ctx(L.OVERVIEW)} title="DST docs" headExtra={overviewHead} bodyEnd={overviewEnd}>
        <P.OverviewPage ctx={ctx(L.OVERVIEW)} hasViewer={hasViewer} />
      </P.Layout>
    ),
  });

  pages.push({
    path: L.COVERAGE,
    element: (
      <P.Layout ctx={ctx(L.COVERAGE)} title="Coverage · DST docs">
        <P.CoveragePage ctx={ctx(L.COVERAGE)} />
      </P.Layout>
    ),
  });

  pages.push({
    path: L.OPERATIONS_INDEX,
    element: (
      <P.Layout ctx={ctx(L.OPERATIONS_INDEX)} title="Operations · DST docs">
        <P.OperationsIndex ctx={ctx(L.OPERATIONS_INDEX)} />
      </P.Layout>
    ),
  });
  for (const op of model.operations) {
    const path = L.operationPath(op);
    pages.push({
      path,
      element: (
        <P.Layout ctx={ctx(path)} title={`${op.name} · DST docs`}>
          <P.OperationPage ctx={ctx(path)} op={op} />
        </P.Layout>
      ),
    });
  }

  pages.push({
    path: L.FAULTS_INDEX,
    element: (
      <P.Layout ctx={ctx(L.FAULTS_INDEX)} title="Faults · DST docs">
        <P.FaultsIndex ctx={ctx(L.FAULTS_INDEX)} />
      </P.Layout>
    ),
  });
  for (const f of model.faults) {
    const path = L.faultPath(f);
    pages.push({
      path,
      element: (
        <P.Layout ctx={ctx(path)} title={`${f.label} · DST docs`}>
          <P.FaultPage ctx={ctx(path)} fault={f} />
        </P.Layout>
      ),
    });
  }

  pages.push({
    path: L.INVARIANTS_INDEX,
    element: (
      <P.Layout ctx={ctx(L.INVARIANTS_INDEX)} title="Invariants · DST docs">
        <P.InvariantsIndex ctx={ctx(L.INVARIANTS_INDEX)} />
      </P.Layout>
    ),
  });
  for (const inv of model.invariants) {
    const path = L.invariantPath(inv);
    pages.push({
      path,
      element: (
        <P.Layout ctx={ctx(path)} title={`${inv.id} ${inv.label} · DST docs`}>
          <P.InvariantPage ctx={ctx(path)} inv={inv} />
        </P.Layout>
      ),
    });
  }

  for (const page of pages) {
    const full = join(outDir, page.path);
    mkdirSync(dirname(full), { recursive: true });
    writeFileSync(full, html(page.element));
  }

  process.stdout.write(
    [
      `Wrote ${pages.length} pages to ${outDir}`,
      hasViewer
        ? `Embedded interactive viewer from ${assetsDir} (${readdirSync(join(outDir, "assets")).join(", ")})`
        : `No viewer island at ${assetsDir} — overview shows a fallback note. Build it with: npm run build:viewer`,
      ``,
    ].join("\n"),
  );
}

main();
