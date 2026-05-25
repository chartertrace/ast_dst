// Public entry point for the DST AST viewer component.
//
// Usage:
//   import { DstAstView, type DstModel } from "@chartertrace/dst-ast-view";
//   import model from "./model.json"; // emitted by the astdst Go tool
//   <DstAstView model={model as DstModel} />

export { DstAstView } from "./DstAstView";
export type { DstAstViewProps } from "./DstAstView";
export { buildGraph, highlightSet } from "./graph";
export type { Graph } from "./graph";
export * from "./types";
