// Flat ESLint config focused on the one thing CI should enforce here:
// accessibility (eslint-plugin-jsx-a11y) plus React hook correctness. It uses
// the typescript-eslint parser so .tsx parses, but deliberately does NOT pull in
// the heavy typescript-eslint rule sets — the goal is an a11y regression gate,
// not a whole-codebase style sweep.
import jsxA11y from "eslint-plugin-jsx-a11y";
import reactHooks from "eslint-plugin-react-hooks";
import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["dist-viewer/**", "node_modules/**", "demo/**", "**/*.d.ts"] },
  {
    files: ["src/**/*.{ts,tsx}"],
    languageOptions: {
      parser: tseslint.parser,
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
    plugins: { "jsx-a11y": jsxA11y, "react-hooks": reactHooks },
    rules: {
      ...jsxA11y.flatConfigs.recommended.rules,
      "react-hooks/rules-of-hooks": "error",
      "react-hooks/exhaustive-deps": "warn",
    },
  },
);
