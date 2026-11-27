# AI Assistant Rules (PART 0, 1)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never modify `AI.md`; it is the implementation source of truth.
- Never guess about requirements, behavior, or paths; search the project and ask when the spec is unclear.
- Never claim completion without checking the actual output and required verification.
- Never leave partial work, silent fixes, or unreported spec gaps.
- Never erase user-owned TODO content or replace project memory with spec text.

## CRITICAL - ALWAYS DO
- Read the relevant AI.md section and project idea before implementation; recheck after context compaction.
- Read a file before editing and search for reusable code before creating new code.
- Finish each scoped task and verify it before moving on; keep documentation synchronized with code.
- Keep `TODO.AI.md` for project work and reserve `TODO.SERVER.md` for contradictions inside AI.md.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|---|---|---|
| Which file defines implementation rules? | AI.md; read-only and authoritative. | PART 0 |
| Which file defines product behavior? | IDEA.md. | PART 0 |
| How are unresolved implementation gaps tracked? | TODO.AI.md. | PART 0 |

For complete details, see AI.md PART 0, 1.
