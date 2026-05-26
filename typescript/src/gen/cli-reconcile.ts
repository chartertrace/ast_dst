// CLI: report drift between the TLA+ spec and the Go checkers, from model.json.
//
//   tsx src/gen/cli-reconcile.ts --in sample/model.json [--strict]
//
// Flags:
//   --in <file>   model.json to read   (default: sample/model.json)
//   --strict      exit 1 when there is a runtime gap (a model-checked invariant
//                 with no Go checker) — wire this into CI.
//
// Stage 1 of the trace-validation roadmap: the cheapest check that keeps the
// spec and the implementation naming the same truths.

import { hasBlockingDrift, reconcile } from "../model/reconcile";
import { parseArgs } from "./args";
import { loadModel, runCli } from "./cli-util";

function main() {
  const args = parseArgs(process.argv.slice(2));
  const model = loadModel(args.in ?? "sample/model.json");
  const r = reconcile(model);

  const out: string[] = [];
  out.push(`Spec ↔ Go reconciliation (${model.source.simPath})`);
  out.push(
    `  ${r.goInvariants} Go invariants · ${r.specInvariants} spec invariants · ${r.validated} validated (model-checked + runtime)`,
  );
  if (r.generatedTotal > 0) {
    out.push(
      `  ${r.generatedTotal} machine-generated · ${r.generatedVerified} TLC-verified (behavioral) · ${r.generatedUnverified.length} unverified drafts`,
    );
  }
  out.push("");

  const section = (title: string, names: string[]) => {
    out.push(`${title}: ${names.length}`);
    for (const n of names) out.push(`  - ${n}`);
    out.push("");
  };

  section(
    "RUNTIME GAP — model-checked in TLA+ but no Go checker (blocking)",
    r.runtimeGap.map((s) => `${s.name}  [${s.specFile}]`),
  );
  section(
    "SPEC ROT — declared in .tla but not in the model-checked .cfg set",
    r.specRot.map((s) => `${s.name}  [${s.specFile}]`),
  );
  section(
    "FOLK WISDOM — Go checker with no TLA+ counterpart (promote or accept)",
    r.folkWisdom.map((i) => `${i.id} ${i.label}`),
  );
  if (r.generatedTotal > 0) {
    section(
      "DRAFTS — machine-generated but not TLC-verified (⚙✗)",
      r.generatedUnverified.map((s) => `${s.name}  [${s.specFile}]`),
    );
  }

  process.stdout.write(out.join("\n") + "\n");

  if (args.strict === "true" && hasBlockingDrift(r)) {
    process.stderr.write(
      `\nFAIL (--strict): ${r.runtimeGap.length} model-checked invariant(s) have no Go checker.\n`,
    );
    process.exit(1);
  }
}

runCli(main);
