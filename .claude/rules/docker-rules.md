# Docker Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never bake secrets or mutable runtime data into an image.
- Never run the production service as root or expose undeclared host ports/paths.
- Never replace the required toolchain/base image without a spec-approved reason.

## CRITICAL - ALWAYS DO
- Build reproducibly with the prescribed multi-stage pattern, minimal runtime image, non-root user, health check, and signal-safe entrypoint.
- Keep image ports, volumes, environment variables, and config paths in sync with the application.
- Test container build and runtime behavior using the documented isolated workflow.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|---|---|---|
| Where does persistent application data live? | In declared runtime volumes, never in the image layer. | PART 27 |
| Which user runs the service? | The least-privileged configured service user. | PART 27 |

For complete details, see AI.md PART 27.
