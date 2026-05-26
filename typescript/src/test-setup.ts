// Vitest setup (test.setupFiles): registers the extra `expect` matchers used by
// the component tests and augments their TypeScript types — DOM assertions
// (@testing-library/jest-dom) and the accessibility audit (vitest-axe). These
// only register matchers / declare types, so they are harmless for node-env
// tests too.
import "@testing-library/jest-dom/vitest";
import * as axeMatchers from "vitest-axe/matchers";
import { expect } from "vitest";

// Runtime: register the axe matcher. (vitest-axe/extend-expect's auto-register
// doesn't take effect under vitest, so extend explicitly.)
expect.extend(axeMatchers);

// Types: vitest reads custom matchers off the `vitest` module's Assertion
// interface (this is how @testing-library/jest-dom augments it too).
declare module "vitest" {
  interface Assertion {
    toHaveNoViolations(): void;
  }
  interface AsymmetricMatchersContaining {
    toHaveNoViolations(): void;
  }
}
