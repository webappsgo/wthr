# Backend Rules (PART 9, 10, 11, 32)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Use bcrypt — Argon2id only for password hashing
- ❌ Store raw tokens — SHA-256 hash before storing
- ❌ Log raw credentials, tokens, or passwords — ever
- ❌ Use `math/rand` for security — `crypto/rand` only
- ❌ Fallback to weak RNG when crypto/rand fails — PANIC instead (fail closed)
- ❌ Use sequential IDs for security-sensitive resources — use UUIDs/crypto tokens
- ❌ Use `ReadAll`/unbounded reads on network streams — use `io.LimitReader`
- ❌ Open DB connections without timeout/deadline
- ❌ Use SQLite in WAL mode without proper cleanup
- ❌ Skip input validation — server validates EVERYTHING
- ❌ Return different errors for "wrong password" vs "user not found" — same message always
- ❌ Use `rand.Seed(time.Now().UnixNano())` — deprecated since Go 1.20
- ❌ Make network calls without timeout/context deadline
- ❌ Spawn goroutines without a cap — use worker pool or semaphore

## CRITICAL - ALWAYS DO
- ✅ Argon2id for all password hashing — parameters: time=1, mem=64MB, threads=4, keyLen=32
- ✅ SHA-256 before storing any token (never store raw tokens)
- ✅ `crypto/rand` for all random values — panic on failure, never degrade
- ✅ Constant-time comparison for secrets (`subtle.ConstantTimeCompare`)
- ✅ Parameterized queries for all DB operations — never string concatenation
- ✅ Rate limiting on all auth endpoints with exponential backoff
- ✅ Audit log for security-relevant events (auth, admin actions, permission changes)
- ✅ Same error message and timing for wrong-password vs no-such-user
- ✅ Context/timeout on every network call, DB query, and subprocess wait
- ✅ Close every file/socket/pipe (defer f.Close())
- ✅ Size-cap all untrusted input (LimitedReader)
- ✅ Tor hidden service auto-detected and auto-enabled if `tor` binary found (PART 32)

## PASSWORD HASHING (Argon2id)
```go
// ALWAYS use these parameters
params := argon2.Params{
    Time:    1,
    Memory:  64 * 1024, // 64 MB
    Threads: 4,
    KeyLen:  32,
}
```

## TOKEN STORAGE PATTERN
```go
// Generate: crypto/rand → hex → store SHA-256(token), return raw token once
// Verify: SHA-256(presented) == stored hash — constant-time comparison
```

## DATABASE
- Primary: SQLite (two files: `server.db` for app data, `users.db` for accounts)
- AIO image: PostgreSQL via Unix socket (`/run/postgresql`)
- Cache: Valkey (Redis-compatible) via Unix socket when AIO, optional otherwise
- All queries: parameterized, with context deadline

## ERROR HANDLING
| Situation | Action |
|-----------|--------|
| Auth failure (any reason) | Generic "invalid credentials" — no enumeration |
| Rate limit exceeded | 429 with Retry-After header |
| Internal error | 500 with request ID, no stack trace in response |
| Not found | 404, but opaque for security-sensitive resources |

---
For complete details, see AI.md PART 9, 10, 11, 32
