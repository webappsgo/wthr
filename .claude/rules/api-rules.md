# API, Health & TLS Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER Add undocumented routes, legacy aliases, ad-hoc response shapes, or incompatible API behavior.
- NEVER Hardcode an API version or hand-edit generated API output.
- NEVER Expose sensitive configuration or weaken TLS/certificate handling.

## CRITICAL - ALWAYS DO

- ALWAYS Keep route inventory, methods, auth, validation, content negotiation, and response schemas aligned with IDEA.md and generated API docs.
- ALWAYS Use shared version/path helpers and the specified health response behavior.
- ALWAYS Follow the prescribed certificate lifecycle and TLS policy.

## Key Rules Summary

Health, API contracts, route structure, and TLS are defined by PARTS 13–15.

For complete details and exact behavior, read AI.md PART 13, 14, 15. AI.md is authoritative; this file is only a topic-scoped quick reference.
