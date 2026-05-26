# Plan: Generate TLC-verified TLA+ specs from a Go codebase

## Context

`ast_dst` today **reads** a hand-written TLA+ spec and binds it to existing Go
invariants (`golang/extract/tla.go`). This plan adds the inverse: a `generate`
layer that **produces** a TLA+ spec from the code, verifies it with TLC, and
feeds the result back through the existing binding + viewer pipeline.

Decisions locked with the user:
- **Deterministic synthesis** (no LLM, no network) — *superseded the original
  LLM-assisted plan; the Anthropic path was built then removed at the user's
  request.*
- **Go-only** for v1 (reuse the `go/ast` pipeline).
- Output must be **TLC-checked** before it is surfaced.
- A **new layer** that reuses the existing extractor + `model.json` + viewer.

### The central difficulty (read first)
TLC needs a *finite, abstract* state model with concrete bounds; real Go state
is unbounded. So the LLM's real job is to produce a sound **abstraction**, and
the verification loop's job is to mechanically reject abstractions that don't
parse or run. "Self-checked" = nothing is labeled `verified` unless TLC actually
model-checked it clean. This preserves the project's "gaps render as gaps" ethos:
a repo with no discernible state machine yields *no* spec, never a hallucinated one.

---

## Status — implemented (v1, deterministic)

Built and green. Stage → shipped files:

- **State model** → `golang/extract/state.go` (`StateModel`: struct fields →
  VARIABLES candidates; interface → opaque single var). ✓
- **Deterministic synthesis** → `gen/synth.go`. Maps Go field types to TLA+
  (`int*`→`Int`, `bool`→`BOOLEAN`, `string`→`STRING`, maps/slices→empty
  function, opaque otherwise) for `VARIABLES`/`Init`/`TypeOK`; one
  `UNCHANGED vars` **stub** per operation; invariant predicates **recovered
  verbatim** from a linked spec, activated only when their identifiers resolve
  to declared vars (else carried as reference comments; no predicate → TODO
  stub). No LLM, no network. ✓
- **Verification** → `gen/toolchain.go` (SANY + TLC runners, auto-located
  `tla2tools.jar`) + `gen/generate.go` (synth → SANY → TLC, one shot). Tested by
  `gen/synth_test.go` (pure synthesis + a real-toolchain verify) and
  `gen/toolchain_test.go` over `gen/testdata/{Good,Bad,Broken}.{tla,cfg}`. ✓
- **Integrate** → `cli/generate.go` (write spec → re-extract with it bound in →
  `extract.MarkGenerated`); viewer provenance badges in
  `typescript/src/components/{util.ts,InvariantTree.tsx,DetailPanel.tsx}`;
  `SpecInvariant` provenance in `golang/extract/model.go` + `typescript/src/types.ts`. ✓
- **CLI** → `astdst generate` (`cmd` & `cmd/astdst` → `cli.Main` →
  `cli.RunGenerate`), flags `--out-spec`, `--module`, `--workers`, `--timeout`, `--jar`. ✓

The **LLM path** (`gen/{llm,prompt,draft}.go` + a repair loop) was built and then
removed at the user's request; its useful pieces — state extraction and the
SANY/TLC toolchain — were kept.

## Remaining & caveats

- **Stage 5 benchmark — not built.** Nothing yet diffs generated invariants
  against the hand-written 50 to produce an accuracy signal; this is the best
  next validation step.
- **Reconciliation (distinct from generation) is built and CI-gated.** The
  spec↔Go drift check — the cheapest "Stage 1" of the broader trace-validation
  roadmap — lives in `typescript/src/model/reconcile.ts` + `gen/cli-reconcile.ts`
  and now runs in CI under `--strict` (fails on a model-checked invariant with no
  Go checker).
- **Trace validation is the next roadmap step and is *not* built.** Emitting
  NDJSON traces from DST runs and replaying them against a `TraceSpec` (Cirstea/
  Kuppe/Loillier/Merz, SEFM 2024; etcd-io/raft PR #113) is what would actually
  test that the Go code *refines* the spec. `gen:trace` already emits a
  `*Trace.tla`/`.cfg` scaffold toward this. Expect **atomicity alignment** (one
  spec step ↔ several program steps) to be the dominant cost.
- **Soundness scope (now stated in the viewer + README).** A generated spec
  marked `verified` means TLC checked the **LLM-chosen abstract model** clean —
  *not* that the Go implementation conforms to it. Likewise runtime `validated`
  means a Go checker exists and is exercised by DST (sampling), not a refinement
  proof.

---

## Architecture (original design): a new `generate` subcommand

Reuses `extract.Model` and the parser. New Go package `golang/gen/` plus one new
extractor file. Five stages:

### Stage 0 — Extract a state model (new: `golang/extract/state.go`)
The current model has ops/faults/invariants but **no VARIABLES**, which TLA+
requires. Add:
- The primary mutable state struct(s) + field names/types (config-named, e.g.
  `state.World`), reusing `findValueSpec`/AST walking already in `parse.go`.
- Per operation handler: a rough **read/write set** over state fields, by
  inspecting `SelectorExpr`s on the state receiver inside the handler body
  (same traversal style as `traceCalls` in `operations.go`).

Emits a `StateModel{Variables []Var, OpEffects map[string]RWSet}` on the model.

### Stage 1 — Build the spec bundle (new: `gen/bundle.go`)
A compact structured JSON the LLM consumes — **not** the raw repo:
- state variables + types,
- operations as candidate guarded transitions (read/write sets, injected faults,
  and the handler body source snippet),
- existing Go invariants as candidate properties to formalize,
- domain hints from config.
Plus a curated set of source snippets (state struct defs, handler bodies). Keeps
tokens bounded and grounds the model in real code.

### Stage 2 — LLM drafts TLA+ (new: `gen/llm.go`)
Anthropic API (Go SDK), **prompt caching** on the static output contract,
structured output. Prompt = bundle + strict contract + a *small fixed catalog of
allowed abstraction patterns* (bound collections to CONSTANT sets of size 1–3;
money/counts as `Nat`; IDs as TLA+ model values; etc.). Emits:
- `Generated.tla`: EXTENDS, CONSTANTS, VARIABLES, `vars`, `TypeOK`, `Init`,
  one Action per op, `Next == \/ ...`, the invariants, and an `AllInvariants`
  aggregator (so the existing `tla.go` declared-invariant detection picks them up).
- `Generated.cfg`: CONSTANT bindings to tiny finite values, `SPECIFICATION Spec`,
  `INVARIANT` lines, optional `CONSTRAINT` to bound the state space.

### Stage 3 — Self-checking verification loop (new: `gen/verify.go`)
Vendor `tla2tools.jar` (auto-download to a cache dir; Java 17 confirmed present).
Loop up to `--max-repairs` (default 5):
1. **SANY** (`tla2sany.SANY`) → syntax/semantic errors.
2. **TLC** (`tlc2.TLC -config Generated.cfg`) with strict bounds (`-workers`,
   depth/state caps, wall-clock timeout) → deadlocks, type errors, invariant
   violations.
3. Classify: clean explored state space ⇒ **verified**; invariant violation w/
   counterexample or error/deadlock ⇒ feed the TLC stderr back to the LLM to
   repair, then re-run.
Record provenance per spec: `generated`, `verified`, iterations used, TLC stats
(states explored, depth).

### Stage 4 — Integrate into the existing model + viewer
Write the verified `.tla`/`.cfg` into a generated spec dir, then run the
**existing** `parseSpecs` → `attachSpecs` (`tla.go`/`extract.go`) pointed at it,
so generated specs flow into `model.json` exactly like hand-written ones. Add
`generated`/`verified`/`tlcStates` to `SpecInvariant` (mirror in `types.ts`).
Viewer badge: "⚙ generated · ✓ verified" vs "⚙ generated · ✗ unverified".

---

## Honesty reconciliation (matches project ethos)
- A generated spec backs an invariant **only** if TLC model-checked it clean.
  Unverified drafts are shown clearly marked, never counted as coverage.
- Surface the soundness gap explicitly: TLC passing proves the *LLM-chosen
  abstract model* satisfies the stated properties — **not** that the Go code is
  correct. State this in the viewer and docs.
- Determinism: golden tests can't cover LLM output. Test the *pipeline* (bundle
  build, SANY/TLC invocation, result parsing, repair loop) with fixtures, and add
  **record/replay** of LLM responses for reproducible CI.

---

## CLI / config
```
go run ./cmd generate --root <repo> --config my.json \
  --out-spec ./generated --model-out model.json --max-repairs 5 --tlc-timeout 60s
```
Needs `ANTHROPIC_API_KEY`, Java, and `tla2tools.jar` (auto-cached). Config gains a
`generate` block: state struct name(s)/package, abstraction size defaults, model name.

## Phasing
1. State extraction + bundle builder (deterministic, fully testable).
2. tla2tools harness — SANY + TLC runners + result parser, tested with known-good
   and known-bad `.tla` fixtures (no LLM yet).
3. LLM drafting + repair loop, with response record/replay.
4. Integration into `model.json` + viewer badges.
5. **Built-in benchmark**: the sim already ships a hand-written spec
   (`../docs/tla`). Generate a spec for the sim and diff generated invariants
   against the existing 50 — overlap is a real accuracy signal.

## Scope reality check
Even LLM-assisted, quality tracks whether the repo has a discernible state
machine. v1 realistically targets Go services/engines with identifiable mutable
state + operations. The generalization win: we drop the catalogue-slice /
weight-table / `recordFault` shape requirement (the LLM reads handler bodies
directly). Libraries/UIs/CLIs with no meaningful state will correctly yield
"no spec generated."

## Files
- New: `golang/extract/state.go`, `golang/gen/{bundle,llm,verify,toolchain}.go`,
  `golang/gen/testdata/*.tla|*.cfg`, viewer badge in `typescript/src/components/`.
- Modified: `golang/cmd/main.go` (subcommand), `golang/extract/{model,config}.go`
  (StateModel + `generate` config + `SpecInvariant` provenance), `typescript/src/types.ts`.

## Verification
- `go test ./...` covers state extraction, fixture-based SANY/TLC parsing, replayed
  LLM repair loop.
- End-to-end: run `generate` against the sim; confirm emitted spec passes TLC and
  that generated invariants overlap the hand-written 50.
