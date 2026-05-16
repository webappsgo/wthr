# API Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Return different HTTP status codes for same error depending on authentication
- ❌ Return stack traces or internal paths in API responses
- ❌ Use sequential IDs in public API — use UUIDs
- ❌ Skip OpenAPI/Swagger annotations — all endpoints must be documented
- ❌ Return 200 OK with error body — use correct HTTP status codes
- ❌ Expose internal DB IDs in API responses
- ❌ Skip rate limiting on any public endpoint
- ❌ Return unbounded lists — always paginate

## CRITICAL - ALWAYS DO
- ✅ RESTful endpoints under `/api/` prefix
- ✅ GraphQL endpoint at `/graphql`
- ✅ OpenAPI/Swagger at `/swagger/` — every endpoint documented
- ✅ Health endpoints: `/health` (liveness), `/health/ready` (readiness), `/health/full` (comprehensive)
- ✅ Version info at `/api/version`
- ✅ Consistent JSON response envelope for errors
- ✅ Pagination for all list endpoints
- ✅ Rate limiting: 20 req/min anonymous, 100 req/min authenticated, unlimited admin
- ✅ CORS headers configured for API endpoints
- ✅ Content-Type: application/json for all API responses
- ✅ Let's Encrypt auto-TLS when domain is configured (PART 15)

## API RESPONSE FORMAT
```json
{
  "data": { ... },
  "error": null,
  "meta": {
    "version": "1.2.3",
    "request_id": "uuid"
  }
}
```

## HEALTH ENDPOINTS
| Endpoint | Returns | Purpose |
|----------|---------|---------|
| `GET /health` | 200/503 | Liveness probe |
| `GET /health/ready` | 200/503 | Readiness probe |
| `GET /health/full` | JSON object | Comprehensive status |

## HTTP STATUS CODE RULES
| Situation | Status |
|-----------|--------|
| Success | 200 OK |
| Created | 201 Created |
| No content | 204 No Content |
| Bad request | 400 Bad Request |
| Unauthorized | 401 Unauthorized |
| Forbidden | 403 Forbidden |
| Not found | 404 Not Found |
| Rate limited | 429 Too Many Requests |
| Server error | 500 Internal Server Error |
| Service unavailable | 503 Service Unavailable |

## TLS / LET'S ENCRYPT
- Auto-enabled when `server.domain` is set and not localhost
- Cert stored in `{data_dir}/certs/`
- Redirect HTTP → HTTPS automatically
- ACME challenge: HTTP-01 at `/.well-known/acme-challenge/`

---
For complete details, see AI.md PART 13, 14, 15
