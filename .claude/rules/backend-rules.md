# Backend Rules (PART 9, 10, 11, 32)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never let a cache, optional integration, or replica replace the configured primary as source of truth.
- Never concatenate untrusted values into SQL, expose internal errors, or log credentials/personal data.
- Never fail startup or enter a restart loop because an optional dependency or overlay network is unavailable.

## CRITICAL - ALWAYS DO
- Use parameterized database operations, transactions, idempotent schema setup, and bounded retries.
- Apply authentication, authorization, rate limits, and privacy-safe audit logging at the prescribed boundaries.
- Keep cluster and mixed-mode replicas subordinate to the shared primary; follow schema and event-order guarantees.
- Keep Tor/I2P behavior aligned with the project’s explicit runtime activation rules.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|---|---|---|
| Which database wins during conflict? | The configured primary; replicas never promote themselves. | PART 10 |
| What does an optional service outage do? | Degrade the relevant feature while the server continues serving available work. | PART 9, 32 |

For complete details, see AI.md PART 9, 10, 11, 32.
