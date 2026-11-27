# Configuration Rules (PART 5, 6, 12)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never hardcode runtime configuration or secrets in source, images, or docs.
- Never log secret values or silently accept invalid configuration.
- Never make an optional dependency failure prevent the server from starting.

## CRITICAL - ALWAYS DO
- Use the canonical configuration schema, defaults, validation, and documented precedence.
- Redact secret values in generated examples and diagnostics; preserve operator-owned settings.
- Keep application mode and server configuration behavior consistent across CLI, config, and deployment docs.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|---|---|---|
| How is configuration validated? | At startup against the typed schema; report actionable errors without exposing secrets. | PART 5 |
| What happens when an optional dependency is unavailable? | The server stays available and reports degraded status. | PART 6, 12 |

For complete details, see AI.md PART 5, 6, 12.
