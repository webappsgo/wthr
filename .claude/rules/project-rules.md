# Project Rules (PART 2, 3, 4)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never add attribution or generated-by credit to project source or documentation.
- Never place source outside the prescribed source tree or invent project structure.
- Never use host-specific filesystem paths in application behavior.

## CRITICAL - ALWAYS DO
- Preserve the existing license and its required notices.
- Keep product definition in IDEA.md and keep README/docs consistent with implemented behavior.
- Use portable, OS-aware paths and the project path helpers.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|---|---|---|
| Where does Go source live? | Under `src/`; the application entry point is `./src`. | PART 3 |
| How are runtime paths formed? | Through the prescribed OS-specific path helpers. | PART 4 |

For complete details, see AI.md PART 2, 3, 4.
