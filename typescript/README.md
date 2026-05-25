# dst-ast-view (local viewer)

A self-contained React component that renders the AST-extracted structure of
the sim DST. It takes one prop — the `model.json` produced by the `astdst` Go
tool in [`../golang`](../golang) — and needs nothing else: styling is bundled
(namespaced under `.dstast`, themed via CSS variables), and `react` /
`react-dom` are peer dependencies.

This viewer is **not published to npm** — it ships as source for local use
(`"private": true`). Run the demo below, or import it by path from another app
in this repo; your bundler compiles the `.tsx`.

## Use it locally

```tsx
import { DstAstView, type DstModel } from "../../ast_dst/typescript/src";
import model from "../../ast_dst/model.json"; // emitted by `go run ./cmd/astdst`

export function DstStructurePage() {
  return (
    <div style={{ height: "100dvh" }}>
      <DstAstView model={model as DstModel} />
    </div>
  );
}
```

The component fills its parent, so give the parent a height.

### Props

| Prop | Type | Notes |
|------|------|-------|
| `model` | `DstModel` | The parsed `model.json`. Required. |
| `className` | `string` | Optional extra class on the root, e.g. to override the `--dstast-*` CSS variables for theming. |

## Layout

Three linked columns plus a detail panel:

- **Operations** — weighted dispatch table; bar = relative weight, chips = the
  faults the handler body injects.
- **Faults** — the 15 fault types; each shows how many ops inject it.
- **Invariants** — 50 properties grouped under their 9 Truths, collapsible.
  Each carries a TLA+ coverage badge: ✓ validated (model-checked), ⚠ specified
  but unchecked, ○ unspecified (no TLA+ counterpart).
- **Detail panel** — the selected node's doc comment and the `file:line` it was
  parsed from. For an invariant, it also shows the TLA+ predicate and spec line
  beside its Go checker.

Click any node to highlight its one-hop neighbourhood (e.g. an op lights up the
faults it injects). Click again, or "clear", to reset.

## Run the demo

```bash
cd ast_dst/typescript
npm i -D vite @vitejs/plugin-react react react-dom @types/react
npx vite demo
```

`demo/main.tsx` mounts `<DstAstView>` with `sample/model.json`.

## Generate from the model

Two generators consume the same `model.json` (no Go changes — everything they
need is already in the model). Install dev deps first: `npm install`.

### Reconciliation (spec ↔ Go drift)

```bash
npm run gen:reconcile -- --in sample/model.json [--strict]
```

The cheapest, highest-leverage check for keeping a TLA+ corpus and its Go
checkers honest (Stage 1 of the trace-validation roadmap — Cirstea/Kuppe/
Loillier/Merz, *Validating Traces of Distributed Programs Against TLA+
Specifications*, SEFM 2024, arXiv:2404.16075). Three sources should name the same
truths: the `.tla` modules (declared), the TLC `.cfg` (model-checked), and the Go
checkers (runtime). From `specStatus` + `specs[].checked` the report derives:

- **runtime gap** — model-checked in TLA+ but no Go checker (the sharpest drift;
  `--strict` exits non-zero on this, so wire it into CI). Conventional type
  invariants (`TypeOK`, `*TypeInvariant`) are excluded — they're never mirrored
  at runtime.
- **spec rot** — declared in a `.tla` module but absent from the `.cfg` checked
  set. Often expected (state-space explosion forces a checked subset).
- **sim-only** — a Go checker with no TLA+ counterpart; promote to the spec or
  knowingly accept (e.g. ECONOMICS).
- **drafts** — when the model carries machine-generated specs (`astdst generate`),
  it also reports how many are TLC-verified (behavioral) vs. unverified `⚙✗`
  drafts.

### Trace-validation scaffold

> Spec **synthesis** (VARIABLES/Init/Next + invariants, verified with SANY/TLC)
> is owned by the Go `astdst generate` command — it has the state model and write
> sets and runs the real tools. This TS generator deliberately does *not*
> re-synthesize a spec; it produces the one piece the Go side doesn't: a TraceSpec
> that replays a DST trace against an existing spec.

```bash
npm run gen:trace -- --in sample/model.json --out tla-gen --base PayoutFlowGen
```

Writes `<base>Trace.tla` + `.cfg` — a **trace-validation scaffold** (Cirstea/Kuppe/
Loillier/Merz, SEFM 2024, arXiv:2404.16075; production reference: etcd-io/raft
PR #113). It `EXTENDS` `--base` (normally the module name `astdst generate`
produced), reads an NDJSON trace via `ndJsonDeserialize(IOEnv.TRACE_PATH)`,
composes one `IsEvent("Op") \cdot Op` per operation, and accepts when TLC has
consumed the whole trace (`TraceAccepted`). Fill in `UpdateVariables` (the one
spot that maps event JSON onto the spec's `VARIABLES`) and have your Go DST
harness emit `{"event":"<OpName>", ...}` lines — then
`TRACE_PATH=trace.ndjson tlc -config <base>Trace.cfg <base>Trace.tla` checks real
runs against the spec.

### Static doc site (godoc-style)

```bash
npm run gen:docs -- --in sample/model.json --out site \
  --repo-base https://github.com/<org>/<repo>/blob/<ref>/primary-server/sim
```

Renders a self-contained static site (`pkg.go.dev`-style): an overview with the
stats, truth list, and TLA+ coverage — plus the **interactive `<DstAstView/>`
embedded as an island** — and per-symbol pages for operations, faults, and
invariants (docs, weights, TLA+ predicate, spec-status badge, source links, and
cross-links derived from the model's edges). Invariants also carry the
`⚙✓`/`⚙~`/`⚙✗` provenance badge for machine-generated specs, a **Coverage** page
reports drift (mirrors `gen:reconcile`), and — when the model has a `state` block —
a **State** page lists the VARIABLES (honestly labelled when the state type is an
interface, not a struct). `gen:docs` first runs `build:viewer`
(Vite) to bundle the island into `dist-viewer/`, then writes pages into `--out`.
`--repo-base` makes `file:line` references clickable; omit it for plain text.
Open `site/index.html` directly or host the folder (e.g. GitHub Pages).

## Typecheck & test

```bash
npm run typecheck   # tsc --noEmit
npm test            # vitest: glyph conversion + TLA+ generation
```

## Keep in sync

`src/types.ts` mirrors `golang/extract/model.go`. If the Go model changes,
update the TS types and regenerate `sample/model.json`.
