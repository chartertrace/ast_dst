# ast_dst — an AST view of a deterministic simulation test

[![CI](https://github.com/chartertrace/ast_dst/actions/workflows/ci.yml/badge.svg)](https://github.com/chartertrace/ast_dst/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/chartertrace/ast_dst/golang.svg)](https://pkg.go.dev/github.com/chartertrace/ast_dst/golang)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A two-part tool that visualises the **static structure** of a sim-shaped Go
codebase: the weighted operations its engine dispatches, the faults each one
injects, and the invariants (grouped by "truth") those exercise. The extractor
is **config-driven** and knows no codebase-specific names by default — they live
in a preset. The bundled preset describes CharterTrace's `primary-server/sim`
(its weighted ops, 15 faults, and 50 invariants across 9 Truths) as a worked
example; point the tool at your own repo with a `--root` and, if needed, a
config.

Nothing here is hand-maintained. A Go program walks the target's own source with
`go/ast` and emits a JSON model; a local React component renders it. Add a fault
or change a weight, regenerate, and the picture updates.

```
<any Go repo>/*.go ──[go/ast]──▶ golang/  ──▶ model.json ──▶ typescript/ (<DstAstView/>)
                      config-driven extractor               importable React component
```

## Generic by config

Every codebase-specific name is configuration (`golang/extract/config.go`,
schema in [`golang/presets/sim.json`](golang/presets/sim.json)). Running with no
`--config` uses a built-in preset (`DefaultConfig()` in Go), kept identical to
that file by a test (`TestPresetMatchesDefault`). Point the tool at a different
repo by writing your own config:

| Concept | Config key | Sim preset value | Notes |
|---------|-----------|------------------|-------|
| packages to parse | `packages` | `catalog,engine,state` | empty ⇒ recurse the whole tree |
| fault catalogue | `faults.structSliceVar` + `idField`/`labelField` | `Faults` (positional `0`/`1`) | field ref is positional (`0`) or keyed (`ID`) |
| fault enum + docs | `faults.enumType`, `enumSkip`, `nameMapVar` | `FaultType`, `faultNames` | optional; enum-only codebases work too |
| invariant catalogue | `invariants.structSliceVar` + fields | `Invariants` (keyed `ID`/`Label`) | |
| truth grouping | `invariants.groupFromIDPrefix` | `true` | `PAYMENT-1` ⇒ truth `PAYMENT` |
| invariant docs | `invariants.docFuncPattern` | `Check{Title}{Num}` | template tokens `{ID}{Prefix}{prefix}{Title}{Num}` |
| operation table | `operations.tableVar` | `opTable` | handler funcs in dispatch order |
| weights | `operations.weightsFunc` + `weightsVar` | `initWeights` / `opWeights` | reads the `[N]float64{...}` literal + per-line notes |
| **trace setup** | `operations.traceCall` + `traceArgIndex` | `recordFault`, arg `0` | **the op→fault edge**; matches any call by final name — `recordFault`, `span.AddEvent`, `tracer.Record`, … |
| **spec coverage** | `tla.specDir` + `cfgFile` + `matchByLabel` | `../docs/tla`, `PayoutFlow.cfg`, `true` | **the spec↔impl layer**; parses the TLA+ `.tla`/`.cfg`, binds each invariant to the TLA+ invariant it validates (Go `Label` == TLA+ name), and classifies coverage. Empty `specDir` ⇒ layer off |

A partial config overrides only the keys it sets; everything else falls back to
the sim preset.

## Install & regenerate the model

```bash
go install github.com/chartertrace/ast_dst/golang/cmd/astdst@latest
```

```bash
cd golang
go test ./...                                              # self-contained: runs against testdata/, no checkout needed
go run ./cmd/astdst --root /path/to/sim --out model.json   # built-in preset names, your codebase
go run ./cmd/astdst --root /path/to/sim --config presets/sim.json --out model.json   # via the preset file
go run ./cmd/astdst --root /path/to/repo --config my.json  # any other codebase
```

Flags: `--root <dir>` (**required**: the codebase to parse; overrides any root
in `--config`), `--config <file>` (default: built-in sim preset), `--out <file>`
(default stdout), `--indent` (default true). Copy the emitted `model.json` to
`typescript/sample/` if you use it as the viewer fixture.

## Generate a TLA+ spec (deterministic)

The inverse of the binding layer below: instead of reading a hand-written spec,
`astdst generate` *synthesises* one — **deterministically, with no LLM and no
network**. The same model in always yields the same spec out. It declares the
TLA+ `VARIABLES` + `Init` + `TypeOK` from the extracted state struct, turns each
operation's **extracted effects** into a transition — a recovered simple form
becomes a **value transition** (`clock' = clock + 1`, `flag' = TRUE`,
`status' = "done"`), and a write whose form isn't recoverable falls back to a
bounded nondeterministic update (`f' \in 0..MaxNat`) — and fills in invariant
predicates **recovered verbatim** from a linked spec. It then **verifies with the
real tools** — SANY (does it parse?) and TLC (do the invariants hold across the
reachable states?).

```bash
cd golang
go run ./cmd generate --out-spec ./generated --out model.json
# flags: --root --config --module --jar --verify --workers --timeout
go run ./cmd doctor   # is the verification toolchain wired up?
```

**Verification is optional, not a prerequisite.** Synthesis is deterministic and
needs nothing but Go, so the spec is *always* produced. When a JVM and
`tla2tools.jar` are present (found via `--jar`, `$TLA2TOOLS_JAR`, or beside the
configured spec dir — no API key), the draft is checked with SANY + TLC and
badged accordingly. When they're absent the run does **not** fail: it writes the
spec marked `⚙✗` **unverified** and points you at `astdst doctor`, which reports
exactly which of java / `tla2tools.jar` is missing. `--verify=false` skips the
toolchain lookup entirely. The generated `.tla`/`.cfg` land in `--out-spec` and
are re-extracted through the same binding layer, so the spec flows into
`model.json` and the viewer like a hand-written one. The badge is
honest about the strength of the check: `⚙✓` **behavioral** (TLC checked an
active invariant against real value transitions), `⚙~` **well-formed** (parses
and holds, but transitions carry no value semantics — a weak claim), or `⚙✗`
unverified. Config gains a `state` block (`typeName`, `package`) naming the
struct whose fields become the `VARIABLES`.

What it does **not** do, by design: invent. A handler writing a field whose type
can't be finitely modelled (a map, slice, struct) keeps that field `UNCHANGED`
with a flagged comment rather than faking a change; an operation that writes no
modellable field is an honest `UNCHANGED vars` stub; an invariant with no
recoverable formal predicate is a `TODO` stub excluded from `AllInvariants`; and
a recovered predicate written in another spec's vocabulary is carried as a
**reference comment**, never activated. The write-set transitions are *which*
fields change, not *how* (the new value is nondeterministic within its bound), so
"verified" remains a bounded claim — *the invariants hold across the modelled
state space* — not a proof the code is correct. Refining a transition's exact
value, or modelling a map/slice field, is the human's remaining job.

## Run it on any repo (including itself)

The default mode looks for a *sim-shaped* codebase (a fault catalogue, op table,
`recordFault` calls). Point it at a repo without those — including `ast_dst`
itself — and it honestly finds nothing. The `structure` mode instead extracts the
**generic** shape of any Go module and renders it in the same viewer:

```bash
cd golang
go run ./cmd/astdst structure --root . --out /tmp/structure.json
# astdst structure: 3 packages, 102 functions, 35 types, 24 vars/consts, 125 edges
```

It maps three disjoint Go declaration kinds onto the viewer's columns:
**functions** (with their signatures and body weight), **types** (each function
links to the types its signature names), and each **package** grouping its
package-level **vars & consts**. No sim config is needed — just `--root` (add
`--exported-only` to drop unexported declarations). Selecting a function
highlights the types it references; the viewer relabels its columns from the
`meta` block the model carries (DST models omit it, so that path is unchanged).

## Use the viewer

See [`typescript/README.md`](typescript/README.md). In short:

```tsx
import { DstAstView, type DstModel } from "@chartertrace/dst-ast-view";
import model from "./model.json";

<DstAstView model={model as DstModel} />
```

Selecting any node highlights its one-hop neighbourhood and shows its doc
comment plus the `file:line` it was parsed from.

## Generate TLA+ and docs from the model

The same `model.json` drives two TypeScript generators (no Go changes — the
model already carries the predicates, weights, and edges they need). See
[`typescript/README.md`](typescript/README.md):

```bash
cd typescript && npm install
npm run gen:reconcile -- --in sample/model.json --strict   # spec↔Go drift (CI gate)
npm run gen:tla       -- --in sample/model.json --out tla-gen   # *.tla + *.cfg + TraceSpec
npm run gen:docs      -- --in sample/model.json --out site      # static godoc-style site
```

- **`gen:reconcile`** is the cheapest, highest-leverage check (Stage 1 of the
  trace-validation roadmap): from `specStatus` + `specs[].checked` it surfaces the
  three drift sets — *runtime gap* (model-checked but no Go checker, the blocking
  one under `--strict`), *spec rot* (declared in `.tla` but not in the `.cfg`
  set), and *sim-only* (Go checker with no TLA+ counterpart).
- **`gen:tla`** emits a TLA+ scaffold that *parses* (SANY syntax phase): invariant
  predicates filled in from the model (verbatim spec predicate, else the glyph-
  converted doc block). Doc predicates that use non-TLA+ math (`|S|` cardinality,
  `∑`) become commented "transcribe by hand" stubs so the module still parses.
  `CONSTANTS`/`VARIABLES`/`Init`/`Next` are `TODO`, so semantic checks and TLC only
  run once you declare the state. Gaps stay gaps — an invariant with no predicate
  becomes a `TRUE` stub, never an invented one. It also writes a `*Trace.tla`/`.cfg`
  **trace-validation scaffold** (Cirstea/Kuppe/Loillier/Merz, SEFM 2024; etcd-io/
  raft PR #113) ready to replay NDJSON DST traces.
- **`gen:docs`** renders a `pkg.go.dev`-style static site (overview + per-symbol
  pages + a coverage/drift report) that embeds the interactive `<DstAstView/>` viewer.

## TLA+ coverage

The sim's invariant checkers *are* the runtime validation of a formal TLA+
spec (`../docs/tla`). When a spec is configured, the extractor parses the
`.tla`/`.cfg`, binds each Go invariant to the TLA+ invariant it enforces, and
labels every invariant with a coverage status:

- **validated** — model-checked in the `.cfg` *and* has a live Go checker;
- **unchecked-spec** — declared in TLA+ but absent from the `.cfg` checked set;
- **unspecified** — a Go checker with no TLA+ counterpart.

These land in `model.Specs`, on each invariant's `spec`/`specStatus`, in
`validates` edges (`inv:… → spec:…`), and in `stats`. The viewer badges each
invariant and shows the TLA+ predicate beside its Go checker. For the sim this
surfaces an honest fact: the formal spec covers 8 truths, the sim validates 9
(ECONOMICS is implementation-only), and only the `.cfg` subset is model-checked.

**What "validated" does and doesn't mean.** *validated* means a model-checked
TLA+ invariant has a live Go checker that DST exercises — that is sampling, not a
proof. It does **not** establish that the Go code *refines* the spec; testing
that is what trace validation (a later roadmap stage: emit NDJSON traces from DST
runs and replay them against a `TraceSpec`) would do. Likewise, a
machine-generated spec marked *verified* means TLC checked the LLM-chosen
**abstract model** clean — not that the implementation conforms to it.

## Honesty

Edges are derived from real call sites and catalogue data, never inferred to
look complete. If an operation injects no fault, it shows "no faults"; if a
doc names no other invariant, no mention edge is drawn. A Go invariant the
TLA+ spec never formalises reads "unspecified" rather than being hidden. Gaps
render as gaps.
