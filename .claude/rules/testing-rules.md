# Testing Rules (PART 29, 30, 31)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never claim tests, lint, docs, accessibility, or browser review passed without running and checking actual output.
- Never use tests that depend on external production state or leave host artifacts behind.
- Never ship untranslated visible strings, inaccessible controls, or undocumented user-facing changes.

## CRITICAL - ALWAYS DO
- Run the project’s prescribed test/lint gates in the required toolchain and record real results.
- Keep unit/integration/browser tests beside the relevant behavior; use the specified coverage and browser matrices.
- Build documentation strictly and keep route, config, and feature docs in sync.
- Test localization fallback, RTL, accessible names, contrast, keyboard use, and reduced motion.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|---|---|---|
| What verifies a changed page? | The full required browser matrix and page-specific behavior checks. | PART 29, 31 |
| What verifies docs? | The strict documented build, not a successful source compile. | PART 30 |

For complete details, see AI.md PART 29, 30, 31.
