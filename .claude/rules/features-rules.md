# Feature Rules (PART 18, 19, 20, 21, 22, 23)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never let email, notifications, scheduler, GeoIP, metrics, backup, or update failures crash the server.
- Never expose secrets, personal data, or unvalidated remote content through feature output.
- Never claim a backup or update succeeded before verifying the resulting artifact/state.

## CRITICAL - ALWAYS DO
- Follow each feature’s configured enablement, provider, retention, retry, and permission rules.
- Make scheduled work bounded, observable, and safe to retry.
- Keep backup/restore atomic and validate integrity before replacing data.
- Keep metrics low-cardinality and avoid sensitive labels.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|---|---|---|
| May optional feature failures stop the server? | No; report degraded state and keep available services running. | PART 18-23 |
| How is a restore accepted? | Only after validation and the documented safe replacement flow. | PART 22 |

For complete details, see AI.md PART 18-23.
