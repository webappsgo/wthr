# Binary Rules (PART 7, 8, 33)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never make the server binary depend on a working directory, shell wrapper, or host-installed runtime.
- Never print secrets in CLI output or accept ambiguous/undocumented flags.
- Never build a client or agent that bypasses the documented server API contract.

## CRITICAL - ALWAYS DO
- Keep the server binary self-contained and implement the documented CLI flags, exit codes, and help.
- Use the project’s standard build metadata and platform naming.
- Keep client and agent behavior within the defined feature scope and API rules.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|---|---|---|
| How are server commands exposed? | Through the documented binary CLI with consistent help and errors. | PART 8 |
| When is a client included? | Follow the universal client requirements and the client/agent PART. | PART 7, 33 |

For complete details, see AI.md PART 7, 8, 33.
