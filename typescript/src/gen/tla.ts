// Generate a TLA+ specification *scaffold* from the extracted model.
//
// The model is not enough to synthesise a runnable spec: the Go sim does not
// expose its state variables or per-operation transitions in a machine-readable
// way. What the model *does* carry is the formal predicate for many invariants —
// either verbatim ASCII in `specs[]` (parsed from real .tla files) or embedded as
// a "TLA+ Specification:" block in an invariant's doc comment. So we emit a
// module that is structurally complete and parseable, with every invariant
// predicate we can recover filled in, and Init / Next / the per-operation
// transitions left as clearly marked TODO stubs for a human to complete.
//
// Honesty (the project's stated principle): an invariant with no recoverable
// predicate becomes a TRUE stub flagged TODO, never an invented predicate.

import type { DstModel, Invariant } from "../types";
import { invariantPredicate, specsByName } from "../model/predicate";
import { asciiTlaClean } from "../model/glyphs";

export interface TlaOptions {
  /** Module name; also the file base name (`<module>.tla`). */
  moduleName?: string;
  /** Standard modules to EXTEND. */
  extend?: string[];
}

export interface TlaReport {
  invariants: number;
  fromSpec: number; // predicate taken from a linked TLA+ spec (ASCII, verbatim)
  fromDoc: number; // clean predicate recovered from the doc comment, emitted live
  docNeedsHand: number; // doc predicate recovered but uses non-TLA+ math — commented
  stubbed: number; // no predicate found — emitted as a TODO stub
  specOnly: number; // spec invariants with no Go checker, surfaced as-is
  operations: number;
}

export interface TlaOutput {
  module: string;
  cfg: string;
  report: TlaReport;
  /** Resolved module name (also the .tla file base name). */
  moduleName: string;
  /** Operation operator names in `model.operations` order — what the TraceSpec
   * composes its events with, so the two modules stay name-aligned. */
  opNames: string[];
}

const DEFAULT_EXTEND = ["Integers", "Sequences", "FiniteSets", "TLC"];

interface InvOp {
  op: string; // TLA+ operator name (unique)
  body: string; // predicate text (already ASCII), or "" for a stub
  // How it is emitted: a live operator ("spec"/"doc"), a commented "transcribe by
  // hand" block when the doc predicate carries non-TLA+ math ("doc-unclean"), or a
  // TRUE TODO stub when no predicate exists ("none").
  emit: "spec" | "doc" | "doc-unclean" | "none";
  inv: Invariant;
}

/** Indent every line of a predicate body by four spaces, under the `==` line. */
function indentBody(body: string): string {
  return body
    .split("\n")
    .map((l) => (l.length ? "    " + l : l))
    .join("\n");
}

/** Make `base` a unique, valid TLA+ identifier within `used`. */
function uniqueName(base: string, used: Set<string>): string {
  let name = base.replace(/[^A-Za-z0-9_]/g, "_") || "Op";
  if (/^[0-9]/.test(name)) name = "_" + name;
  let candidate = name;
  let n = 2;
  while (used.has(candidate)) candidate = `${name}_${n++}`;
  used.add(candidate);
  return candidate;
}

/**
 * Build a TLA+ module scaffold and a matching TLC config from the model.
 */
export function generateTla(model: DstModel, opts: TlaOptions = {}): TlaOutput {
  const moduleName = (opts.moduleName ?? "DstSpec").replace(/[^A-Za-z0-9_]/g, "");
  const extend = opts.extend ?? DEFAULT_EXTEND;
  const byName = specsByName(model.specs);

  const used = new Set<string>();
  const report: TlaReport = {
    invariants: model.invariants.length,
    fromSpec: 0,
    fromDoc: 0,
    docNeedsHand: 0,
    stubbed: 0,
    specOnly: 0,
    operations: model.operations.length,
  };

  // One operator per invariant, grouped by truth in source order. A doc-recovered
  // predicate is only emitted live if it is clean ASCII TLA+; otherwise it carries
  // non-TLA+ math (e.g. |S| cardinality, ∑) and is downgraded to a commented stub
  // so the module still parses.
  const invOps: InvOp[] = [];
  const usedSpecNames = new Set<string>();
  for (const inv of model.invariants) {
    const { text, source } = invariantPredicate(inv, byName);
    let emit: InvOp["emit"];
    if (source === "spec") {
      emit = "spec";
      report.fromSpec++;
    } else if (source === "doc") {
      if (asciiTlaClean(text)) {
        emit = "doc";
        report.fromDoc++;
      } else {
        emit = "doc-unclean";
        report.docNeedsHand++;
      }
    } else {
      emit = "none";
      report.stubbed++;
    }
    if (inv.spec) usedSpecNames.add(inv.spec.name);
    invOps.push({ op: uniqueName(inv.label, used), body: text, emit, inv });
  }

  // Spec invariants with no Go checker — surface them so nothing is dropped.
  const specOnly = (model.specs ?? []).filter((s) => !usedSpecNames.has(s.name));
  report.specOnly = specOnly.length;

  const L: string[] = [];
  const w = (s = "") => L.push(s);

  // ---- module header --------------------------------------------------------
  w(`---- MODULE ${moduleName} ----`);
  w(`\\* GENERATED by ast_dst from ${model.source.simPath} @ ${model.generatedAt}.`);
  w(`\\* This is a SCAFFOLD: invariant predicates are filled in from the model;`);
  w(`\\* CONSTANTS, VARIABLES, Init, and the per-operation transitions are TODO.`);
  w(`EXTENDS ${extend.join(", ")}`);
  w();
  w(`\\* TODO: declare the model parameters the predicates below refer to, e.g.`);
  w(`\\*   CONSTANTS Drivers, Shipments, MaxBalance`);
  w(`\\* TODO: declare the state the operations mutate, e.g.`);
  w(`\\*   VARIABLES driverBalance, shipmentStatus, ledgerEntries`);
  w();
  // `vars` lets the stub actions type-check as [Next]_vars without a real state
  // declaration yet; replace with the tuple of your VARIABLES.
  w(`vars == << >>  \\* TODO: the tuple of all VARIABLES`);
  w();

  // ---- invariants -----------------------------------------------------------
  w(`\\* ============================================================`);
  w(`\\* Invariants — ${report.fromSpec + report.fromDoc} live, ${report.docNeedsHand} need hand-transcription, ${report.stubbed} TODO`);
  w(`\\* ============================================================`);
  let currentTruth = "";
  for (const io of invOps) {
    if (io.inv.truth !== currentTruth) {
      currentTruth = io.inv.truth;
      w();
      w(`\\* ----- ${currentTruth} -----`);
    }
    w();
    const loc = io.inv.loc ? `${io.inv.loc.file}:${io.inv.loc.line}` : "unknown";
    w(`\\* ${io.inv.id} (${io.inv.specStatus ?? "no-spec"}); source: ${loc}`);
    if (io.emit === "none") {
      w(`${io.op} == TRUE  \\* TODO: no predicate in model for ${io.inv.id}`);
    } else if (io.emit === "doc-unclean") {
      // Recovered from the doc comment but contains non-TLA+ math (e.g. |S|, ∑).
      // Shown verbatim in a comment for a human to transcribe; stubbed so it parses.
      w(`\\* TODO: transcribe to TLA+ (uses non-TLA+ math, e.g. |S| -> Cardinality(S)):`);
      for (const line of io.body.split("\n")) w(`\\*   ${line}`);
      w(`${io.op} == TRUE`);
    } else {
      w(`${io.op} ==`);
      w(indentBody(io.body));
    }
  }

  // ---- spec-only invariants -------------------------------------------------
  if (specOnly.length) {
    w();
    w(`\\* ----- TLA+ spec invariants with no Go checker -----`);
    for (const s of specOnly) {
      const op = uniqueName(s.name, used);
      w();
      w(`\\* from ${s.specFile}${s.checked ? " (model-checked)" : ""}`);
      w(`${op} ==`);
      w(indentBody(s.predicate.trim()));
    }
  }

  // ---- operations / Next ----------------------------------------------------
  w();
  w(`\\* ============================================================`);
  w(`\\* Operations — ${report.operations} weighted actions (transitions are TODO)`);
  w(`\\* ============================================================`);
  const opNames: string[] = [];
  for (const op of model.operations) {
    const name = uniqueName(op.name, used);
    opNames.push(name);
    const loc = op.loc ? `${op.loc.file}:${op.loc.line}` : "unknown";
    const faults = op.faults.length ? `; injects ${op.faults.join(", ")}` : "";
    w();
    w(`\\* weight ${op.weight} (${(op.share * 100).toFixed(1)}%)${faults}; source: ${loc}`);
    w(`${name} ==`);
    w(`    \\* TODO: define this transition`);
    w(`    UNCHANGED vars`);
  }

  // ---- spec scaffolding -----------------------------------------------------
  w();
  w(`Init == TRUE  \\* TODO: define the initial state`);
  w();
  w(`Next ==`);
  if (opNames.length) {
    opNames.forEach((n) => w(`    \\/ ${n}`));
  } else {
    w(`    UNCHANGED vars`);
  }
  w();
  w(`Spec == Init /\\ [][Next]_vars`);
  w();

  // AllInvariants conjoins only invariants emitted as live ASCII predicates.
  const checkable = invOps
    .filter((io) => io.emit === "spec" || io.emit === "doc")
    .map((io) => io.op);
  w(`\\* Conjunction of every invariant emitted as a live predicate.`);
  w(`AllInvariants ==`);
  if (checkable.length) {
    checkable.forEach((n) => w(`    /\\ ${n}`));
  } else {
    w(`    TRUE`);
  }
  const stubs = invOps
    .filter((io) => io.emit === "none" || io.emit === "doc-unclean")
    .map((io) => io.op);
  if (stubs.length) {
    w(`\\* Not yet checkable (TODO / hand-transcribe): ${stubs.join(", ")}`);
  }
  w();
  w(`====`);

  const module = L.join("\n") + "\n";

  // ---- TLC config -----------------------------------------------------------
  const cfg =
    [
      `\\* GENERATED by ast_dst — fill in CONSTANT bindings before model-checking.`,
      `SPECIFICATION Spec`,
      `INVARIANT AllInvariants`,
      `\\* TODO: bind CONSTANTS, e.g.`,
      `\\* CONSTANTS`,
      `\\*   Drivers = {d1, d2}`,
      `\\*   Shipments = {s1, s2}`,
    ].join("\n") + "\n";

  return { module, cfg, report, moduleName, opNames };
}

export interface TraceSpecOptions {
  /** The generated scaffold module to EXTEND. */
  baseModule: string;
  /** Operation operator names in that module, aligned to `model.operations`. */
  opNames: string[];
  /** Trace module name; default `<baseModule>Trace`. */
  traceModuleName?: string;
}

export interface TraceSpecOutput {
  module: string;
  cfg: string;
}

/**
 * Generate a trace-validation scaffold: a module that replays an NDJSON trace
 * (emitted by the Go DST harness) against the spec, following the recipe from
 * "Validating Traces of Distributed Programs Against TLA+ Specifications"
 * (Cirstea/Kuppe/Loillier/Merz, SEFM 2024, arXiv:2404.16075) and the production
 * reference in etcd-io/raft PR #113. Each operation becomes an event that is
 * composed (`\cdot`) with the spec action it linearizes to. `UpdateVariables`
 * is left as a TODO — it is the one place a human must map the event's JSON
 * fields onto the spec's VARIABLES.
 */
export function generateTraceSpec(model: DstModel, opts: TraceSpecOptions): TraceSpecOutput {
  const name = opts.traceModuleName ?? `${opts.baseModule}Trace`;
  const events = model.operations.map((op, i) => ({
    event: op.name, // the "event" string the Go TraceLogger should emit
    baseOp: opts.opNames[i] ?? op.name,
    onOp: `On${(opts.opNames[i] ?? op.name).replace(/[^A-Za-z0-9_]/g, "")}`,
  }));

  const L: string[] = [];
  const w = (s = "") => L.push(s);

  w(`---- MODULE ${name} ----`);
  w(`\\* GENERATED by ast_dst — trace-validation scaffold.`);
  w(`\\* Technique: Cirstea/Kuppe/Loillier/Merz, SEFM 2024 (arXiv:2404.16075);`);
  w(`\\* reference implementation: etcd-io/raft PR #113 (NDJSON, one event/line).`);
  w(`\\* Replays a Go DST trace against ${opts.baseModule}. Run with:`);
  w(`\\*   TRACE_PATH=trace.ndjson tlc -config ${name}.cfg ${name}.tla`);
  w(`EXTENDS ${opts.baseModule}, TLC, Sequences, Integers, Json, IOUtils`);
  w();
  w(`\\* The trace: newline-delimited JSON, one event per line, from $TRACE_PATH.`);
  w(`Trace == ndJsonDeserialize(IOEnv.TRACE_PATH)`);
  w();
  w(`VARIABLE l  \\* 1-based index of the next trace line to consume`);
  w();
  w(`\\* TODO: map the current trace line's fields onto the spec's VARIABLES, e.g.`);
  w(`\\*   /\\ driverBalance' = Trace[i].driverBalance`);
  w(`\\*   /\\ UNCHANGED <<...the rest...>>`);
  w(`UpdateVariables(i) == TRUE`);
  w();
  w(`\\* Consume one trace line, requiring its "event" field (when present) to`);
  w(`\\* match e, then advance.`);
  w(`IsEvent(e) ==`);
  w(`    /\\ l \\in 1..Len(Trace)`);
  w(`    /\\ ("event" \\in DOMAIN Trace[l] => Trace[l].event = e)`);
  w(`    /\\ UpdateVariables(l)`);
  w(`    /\\ l' = l + 1`);
  w();
  w(`\\* One disjunct per operation. The `+"`\\cdot`"+` (action composition) operator aligns`);
  w(`\\* one trace event with the spec action it linearizes to, even when the grain`);
  w(`\\* of atomicity differs between trace and spec.`);
  for (const e of events) {
    w(`${e.onOp} == IsEvent("${e.event}") \\cdot ${e.baseOp}`);
  }
  w();
  if (events.length) {
    w(`TraceNext ==`);
    events.forEach((e) => w(`    \\/ ${e.onOp}`));
  } else {
    w(`TraceNext == UNCHANGED <<l, vars>>`);
  }
  w();
  w(`TraceInit == l = 1 /\\ Init`);
  w(`TraceSpec == TraceInit /\\ [][TraceNext]_<<l, vars>>`);
  w();
  w(`\\* Accept iff TLC consumed exactly the whole trace (no more, no fewer steps).`);
  w(`TraceAccepted == Len(Trace) = TLCGet("stats").diameter - 1`);
  w();
  w(`====`);

  const cfg =
    [
      `\\* GENERATED by ast_dst — trace validation.`,
      `\\* Run: TRACE_PATH=trace.ndjson java -jar tla2tools.jar -config ${name}.cfg ${name}.tla`,
      `SPECIFICATION TraceSpec`,
      `INVARIANT AllInvariants`,
      `POSTCONDITION TraceAccepted`,
      `\\* TODO: bind CONSTANTS to the trace's universe (often derivable from it).`,
    ].join("\n") + "\n";

  return { module: L.join("\n") + "\n", cfg };
}
