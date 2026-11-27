# Makefile Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never use the Makefile as CI/CD orchestration or bypass the prescribed project toolchain.
- Never leave build artifacts in source-controlled paths or run destructive broad cleanup.

## CRITICAL - ALWAYS DO
- Keep local build, test, dev, release, and cleanup targets aligned with the documented project variables and Docker toolchain.
- Use bounded commands, named ephemeral containers, and persistent project-scoped module/build caches.
- Keep target behavior and documentation accurate; test the target after changing it.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|---|---|---|
| Does Makefile define CI? | No; CI belongs in provider workflows. | PART 26, 28 |
| Where do build/test artifacts go? | In designated ignored or project-scoped temporary directories. | PART 26 |

For complete details, see AI.md PART 26.
