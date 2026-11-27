# Service Rules (PART 24, 25)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never run the application with elevated privileges as a substitute for correct service permissions.
- Never make service installation or support commands silently change host configuration.

## CRITICAL - ALWAYS DO
- Follow least privilege and the prescribed privilege-escalation boundary for narrowly scoped operations.
- Keep service manager files, service commands, paths, and support behavior consistent with the supported platforms.
- Make service failures actionable without exposing secrets.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|---|---|---|
| Should the server run as root? | No; use the prescribed service account and narrowly scoped operations. | PART 24 |
| Where is service behavior defined? | In the platform support rules and generated service files. | PART 25 |

For complete details, see AI.md PART 24, 25.
