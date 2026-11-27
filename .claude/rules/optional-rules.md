# Optional Feature Rules (PART 34, 35, 36)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never implement or activate optional multi-user, organization, or custom-domain behavior unless the project declares it.
- Never weaken tenant isolation or infer a tenant from untrusted request data.
- Never present an optional feature as active when its declaration or configuration is absent.

## CRITICAL - ALWAYS DO
- When a PART is implemented or declared, follow every requirement in that PART; “optional” does not relax implementation rules.
- Enforce ownership and authorization on every data access and route.
- Keep optional feature declarations and documentation synchronized with IDEA.md and runtime behavior.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|---|---|---|
| Does optional mean loosely implemented? | No; it means the feature is only activated when declared, and then all rules are mandatory. | PART 34-36 |
| How is tenant context trusted? | Only from the validated authenticated context defined by the relevant PART. | PART 34-36 |

For complete details, see AI.md PART 34-36.
