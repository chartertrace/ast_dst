// CLI: write a trace-validation scaffold (<module>Trace.tla + .cfg) for a spec.
//
//   tsx --tsconfig tsconfig.gen.json src/gen/cli-trace.ts \
//     --in sample/model.json --out tla-gen --base PayoutFlowGen
//
// Flags:
//   --in <file>     model.json to read           (default: sample/model.json)
//   --out <dir>     output directory             (default: tla-gen)
//   --base <name>   spec module to EXTEND        (default: DstSpec)
//                   — normally the module name produced by `astdst generate`.
//   --module <name> trace module name            (default: <base>Trace)
//
// Spec synthesis lives in the Go `astdst generate` (it has the state model and
// runs SANY/TLC). This produces the complementary piece: a TraceSpec that replays
// an NDJSON DST trace against that spec.

import { mkdirSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { generateTraceSpec } from "./trace";
import { parseArgs } from "./args";
import { loadModel, runCli } from "./cli-util";

function main() {
  const args = parseArgs(process.argv.slice(2));
  const model = loadModel(args.in ?? "sample/model.json");
  const outDir = resolve(args.out ?? "tla-gen");

  const { module, cfg, moduleName } = generateTraceSpec(model, {
    baseModule: args.base,
    traceModuleName: args.module,
  });

  mkdirSync(outDir, { recursive: true });
  const tla = join(outDir, `${moduleName}.tla`);
  const cfgPath = join(outDir, `${moduleName}.cfg`);
  writeFileSync(tla, module);
  writeFileSync(cfgPath, cfg);

  process.stdout.write(
    [
      `Wrote ${tla}`,
      `Wrote ${cfgPath}`,
      ``,
      `${model.operations.length} operation events composed against ${args.base ?? "DstSpec"}.`,
      `Next: fill in UpdateVariables, emit {"event":"<OpName>",...} NDJSON from the DST harness,`,
      `then: TRACE_PATH=trace.ndjson tlc -config ${moduleName}.cfg ${moduleName}.tla`,
      ``,
    ].join("\n"),
  );
}

runCli(main);
