# Testing, Documentation & I18N Rules (PART 29, 30, 31)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER Put runtime/test artifacts in the repository or bypass required authentication in tests.
- NEVER Hardcode user-facing text or omit a locale key from non-English language files.
- NEVER Leave required documentation pages or route/API documentation stale.

## CRITICAL - ALWAYS DO

- ALWAYS Use the prescribed test phases, isolated execution, route coverage, and cleanup behavior.
- ALWAYS Keep README, MkDocs, API docs, IDEA.md, and translations synchronized with implemented behavior.
- ALWAYS Preserve accessibility requirements and verify UI behavior with the specified browser matrix when required.

## Key Rules Summary

Testing, ReadTheDocs documentation, localization, and accessibility are defined by PARTS 29–31.

For complete details and exact behavior, read AI.md PART 29, 30, 31. AI.md is authoritative; this file is only a topic-scoped quick reference.
