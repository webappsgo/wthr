# CI/CD Rules (PART 28)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never use floating third-party workflow action tags where a full commit SHA is required.
- Never expose secrets to untrusted pull requests or grant workflows broader permissions than needed.
- Never report completion while a pushed CI run is pending or failing.

## CRITICAL - ALWAYS DO
- Keep each supported provider’s workflow equivalent and aligned with actual build/test targets.
- Run security, dependency, secret, and workflow-policy checks as required; use least-privilege permissions.
- Validate workflow syntax and check the real provider run after every push; fix failures before completion.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|---|---|---|
| Is local workflow validation the same as CI? | No; provider CI is the final verification. | PART 28 |
| How are external actions referenced? | By verified full commit SHA with a version comment. | PART 28 |

For complete details, see AI.md PART 28.
