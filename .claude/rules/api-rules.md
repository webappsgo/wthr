# API Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never change a public route, response, or error shape without updating OpenAPI/docs and compatibility tests.
- Never return HTML from an API route or JSON from an HTML route outside the specified negotiation rules.
- Never disable TLS verification as a workaround.

## CRITICAL - ALWAYS DO
- Use the canonical versioned route structure, request IDs, content negotiation, and RFC-compliant error responses.
- Keep health/version endpoints and API docs in their prescribed locations.
- Validate TLS certificates and use the configured ACME/Let's Encrypt lifecycle.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|---|---|---|
| How are API errors returned? | In the documented problem response format with request ID propagation. | PART 14 |
| Where are browser docs served? | Under the canonical `/server/docs/` routes. | PART 13, 14 |

For complete details, see AI.md PART 13, 14, 15.
