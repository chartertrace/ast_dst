// CLI: read a model.json and write a TLA+ scaffold (<module>.tla + <module>.cfg).
//
//   tsx src/gen/cli-tla.ts --in sample/model.json --out ../tla-gen --module DstSpec
//
// Flags:
//   --in <file>       model.json to read            (default: sample/model.json)
//   --out <dir>       output directory              (default: tla-gen)
//   --module <name>   TLA+ module / file base name  (default: DstSpec)
//   --no-trace        skip the trace-validation scaffold (<module>Trace.tla/.cfg)
//
// Prints a coverage report so it is obvious how much of the spec is real
// predicate vs. TODO stub.

import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import type { DstModel } from "../types";
import { generateTla, generateTraceSpec } from "./tla";
import { parseArgs } from "./args";

function main() {
  const args = parseArgs(process.argv.slice(2));
  const inPath = resolve(args.in ?? "sample/model.json");
  const outDir = resolve(args.out ?? "tla-gen");
  const moduleName = args.module ?? "DstSpec";

  const model = JSON.parse(readFileSync(inPath, "utf8")) as DstModel;
  const { module, cfg, report, opNames } = generateTla(model, { moduleName });

  mkdirSync(outDir, { recursive: true });
  const written: string[] = [];
  const write = (file: string, data: string) => {
    const p = join(outDir, file);
    writeFileSync(p, data);
    written.push(p);
  };

  write(`${moduleName}.tla`, module);
  write(`${moduleName}.cfg`, cfg);

  if (args["no-trace"] !== "true") {
    const trace = generateTraceSpec(model, { baseModule: moduleName, opNames });
    write(`${moduleName}Trace.tla`, trace.module);
    write(`${moduleName}Trace.cfg`, trace.cfg);
  }

  process.stdout.write(
    [
      ...written.map((p) => `Wrote ${p}`),
      ``,
      `Invariants:   ${report.invariants}`,
      `  from spec:  ${report.fromSpec}  (verbatim ASCII predicate, live)`,
      `  from doc:   ${report.fromDoc}  (recovered from doc comment, live)`,
      `  hand-xlate: ${report.docNeedsHand}  (doc predicate uses non-TLA+ math — commented)`,
      `  TODO stub:  ${report.stubbed}  (no predicate in model)`,
      `Spec-only:    ${report.specOnly}  (TLA+ invariants with no Go checker)`,
      `Operations:   ${report.operations}  (transitions are TODO stubs)`,
      ``,
    ].join("\n"),
  );
}

main();
