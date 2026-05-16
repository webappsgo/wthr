# AI Rules (PART 0, 1)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Guess or assume — READ THE SPEC or ASK
- ❌ Implement without reading relevant PART first
- ❌ Modify AI.md PART 0-33 content (read-only spec; PARTS 34-36 only flip: OPTIONAL→REQUIRED)
- ❌ Skip reading PART 0 and 1 at conversation start
- ❌ Add features not in spec without asking
- ❌ Use "I think" or "probably" — KNOW from spec or ASK
- ❌ Ask multiple plain-text questions in separate messages — use AskUserQuestion wizard
- ❌ Use generic placeholder content ("Your app name", "Feature 1")
- ❌ Create /server/about or /server/help with placeholder text
- ❌ Leave TODO comments in code — implement fully or don't implement
- ❌ Create stub functions or "future" placeholders
- ❌ Partial implementations — every feature must be 100% complete
- ❌ Create premium tiers — all features free, no paywalls
- ❌ Use bcrypt — use Argon2id only
- ❌ Enable CGO — CGO_ENABLED=0 always
- ❌ Put Dockerfile in repo root — docker/Dockerfile only
- ❌ Use external cron/systemd timers — built-in scheduler only
- ❌ Use client-side rendering (React, Vue) — server-side Go templates only
- ❌ Edit IDEA.md `## Project variables` without user confirmation

## CRITICAL - ALWAYS DO
- ✅ Read AI.md PART 0, 1 at start of EVERY conversation
- ✅ Read relevant PART before implementing ANY feature
- ✅ Search AI.md before asking questions (answer is likely there)
- ✅ Follow spec EXACTLY — no "improvements" without approval
- ✅ Update IDEA.md when features change
- ✅ Keep all docs in sync with code
- ✅ When unsure, ASK — never guess or assume
- ✅ Use AskUserQuestion wizard — one question at a time, options + custom input
- ✅ Source /server/about and /server/help content from IDEA.md
- ✅ Implement features 100% complete — no stubs, no TODOs, no "future"
- ✅ ONE thing at a time — finish current task completely before starting another
- ✅ Migrate non-conforming IDEA.md before doing any other work

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| What password hash? | Argon2id (NEVER bcrypt) | PART 11 |
| Where is Dockerfile? | `docker/Dockerfile` (NEVER root) | PART 27 |
| CGO enabled? | NEVER (CGO_ENABLED=0 always) | PART 7 |
| Premium features? | NEVER (all features free) | PART 1 |
| External cron? | NEVER (built-in scheduler) | PART 19 |
| Client-side rendering? | NEVER (server-side Go templates) | PART 16 |
| Ask to continue? | NO — continue until blocked or user asks to pause | PART 0 |
| Multiple questions? | Use AskUserQuestion wizard, not plain text | PART 0 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| server | Main binary `weather` — runs as service |
| client | CLI binary `weather-cli` — REQUIRED companion |
| agent | Optional binary `weather-agent` |
| Server Admin | App administrator (NOT OS root) |
| Primary Admin | First admin — cannot be deleted |
| Regular User | End-user (PART 34, optional feature) |
| Cluster Node | Another weather instance (horizontal scaling) |

## COMPLIANCE CHECK
Before completing ANY task:
- [ ] Read relevant PART(s) in AI.md
- [ ] Implementation matches spec EXACTLY
- [ ] No guessing — all decisions from spec
- [ ] Docs updated if code changed
- [ ] No TODO/FIXME/HACK in committed code

---
For complete details, see AI.md PART 0, 1
