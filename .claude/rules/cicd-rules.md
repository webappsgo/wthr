# CI/CD Rules (PART 28)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER Use unsupported providers, unpinned third-party actions, excessive permissions, or secrets in untrusted jobs.
- NEVER Call Makefile targets in CI when explicit commands are required.
- NEVER Claim CI coverage for build/test artifacts that the repository does not produce.

## CRITICAL - ALWAYS DO

- ALWAYS Build and test the actual project with the prescribed environment and least privilege.
- ALWAYS Pin external actions and keep workflow triggers, permissions, caches, and release steps within PART 28.
- ALWAYS Update workflows when project binaries, tests, or packaging change.

## Key Rules Summary

CI/CD workflow behavior is defined by PART 28.

For complete details and exact behavior, read AI.md PART 28. AI.md is authoritative; this file is only a topic-scoped quick reference.
