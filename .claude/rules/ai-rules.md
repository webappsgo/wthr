# AI Rules (PART 0, 1)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Guess or assume values — run the command or read the spec
- Modify AI.md (READ-ONLY template)
- Create patterns not in spec
- Create report/analysis files (fix directly; only `AUDIT.AI.md` allowed for explicit audits)
- Add unrequested features
- Read an image larger than 1000×1000 directly (resize first)
- Skip the mandatory verification steps before claiming "done"
- Treat a non-conforming IDEA.md as authoritative without migrating it first
- Edit `## Project variables` in IDEA.md without confirming with user
- Use bcrypt → use Argon2id for passwords, SHA-256 for tokens
- Use CGO → CGO_ENABLED=0 always
- Use external cron → internal scheduler (PART 19)
- Use client-side rendering (React/Vue) → server-side Go templates only
- Skip platform builds → build all 8 (linux/darwin/windows/freebsd × amd64/arm64)

## CRITICAL - ALWAYS DO

- Read the relevant AI.md PART before implementing anything
- Ask if unsure; never guess
- Test changes before claiming completion
- Verify output matches expectations
- Search before creating (check if it exists first)
- Read file before editing it
- Surface issues in response before fixing
- Complete current task before starting the next
- Run `make test` before every commit — no exceptions
- Run `go-lint` before committing any Go change
- Use `gitcommit --dir {dir} all` — never `git commit`

## Key Rules

### IDEA.md Three-Section Format (required)

```bash
grep -cE '^## Project description[[:space:]]*$' IDEA.md
grep -cE '^## Project variables[[:space:]]*$' IDEA.md
grep -cE '^## Business logic[[:space:]]*$' IDEA.md
```

All three must return `1`. If not, migrate before proceeding.

### Session Start

1. Read existing `CLAUDE.md` if present
2. Check if `.claude/rules/` directory exists; create/update if stale
3. Read `TODO.AI.md` if present

### Task → PART Reference

| Task | PART |
|------|------|
| Admin auth | 17 |
| Multi-user | 34 |
| Frontend/UI | 16 |
| API endpoints | 14 |
| Tests | 29 |
| Docker | 27 |
| Config | 5 |
| CLI | 8 |
| Translation/i18n | 31 |

## Reference

For complete details, see AI.md PART 0, 1
