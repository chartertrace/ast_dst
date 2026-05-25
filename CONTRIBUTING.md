# Contributing

Thanks for your interest in `ast_dst`. It's two pieces — a Go extractor/generator
(`golang/`) and a local React viewer + model-driven generators (`typescript/`).
Both run in CI on every PR; please make sure the same checks pass locally.

## Go (`golang/`)

```bash
cd golang
gofmt -l .        # must print nothing
go vet ./...
go test ./...     # self-contained — runs against testdata/, no external checkout
```

- The extractor is config-driven; don't hard-code codebase-specific names.
  `DefaultConfig()` is the bundled sim preset and is kept identical to
  `presets/sim.json` by `TestPresetMatchesDefault` — change both together.
- Tests must stay self-contained (use `golang/extract/testdata/`), so a fresh
  clone passes with zero external dependencies.
- `astdst generate` needs `ANTHROPIC_API_KEY` + a JVM + `tla2tools.jar`; its
  pipeline is tested with a canned `replayModel`, so unit tests need none of those.

## TypeScript (`typescript/`)

```bash
cd typescript
npm install
npm run typecheck   # browser (tsconfig.json) + node tooling (tsconfig.node.json)
npm test            # vitest
npm run gen:reconcile -- --in sample/model.json --strict   # spec↔Go drift gate
```

- The shippable viewer lives outside `src/gen/`; Node-only tooling lives in
  `src/gen/` and is typechecked by `tsconfig.node.json`. Keep that split.
- `src/types.ts` mirrors `golang/extract/model.go` — update both when the model
  changes, and regenerate `sample/model.json`.

## Pull requests

- Keep changes focused; describe what and why.
- Run the checks above; CI runs them on Go `1.21` + `stable` and Node `20`.
- Be honest in the model and viewer: gaps render as gaps — never infer an edge or
  mark an invariant `validated`/`verified` that the source doesn't support.

By contributing you agree your work is licensed under the [MIT License](LICENSE).
