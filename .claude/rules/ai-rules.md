# AI Rules (PART 0, 1)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Guess or assume - READ THE SPEC or ASK
- ❌ Implement without reading relevant PART first
- ❌ Modify AI.md PARTS 0-37 (read-only spec; only PART 34-36 OPTIONAL→REQUIRED transitions allowed)
- ❌ Replace placeholder tokens (`{project_name}`, `{PROJECT_NAME}`, `{project_org}`, `{PROJECT_ORG}`, `{project_repo}`, `{official_site}`, `{internal_name}`, `{plist_label}`, etc.) inside `AI.md` — they stay as literal `{...}` tokens forever
- ❌ Skip reading PART 0 and 1 at conversation start
- ❌ Add features not in spec without asking
- ❌ Use "I think" or "probably" - KNOW from spec or ASK
- ❌ Ask multiple plain-text questions in separate messages - use AskUserQuestion wizard
- ❌ Use generic placeholder content ("Your app name", "Feature 1")
- ❌ Create /server/about or /server/help with placeholder text
- ❌ Leave TODO comments in code - implement fully or don't implement
- ❌ Create stub functions or "future" placeholders
- ❌ Partial implementations - every feature must be 100% complete
- ❌ Run `git add`, `git commit`, `git push` - write `.git/COMMIT_MESS` instead
- ❌ Add AI attribution anywhere (code, comments, commits, PRs, docs)
- ❌ Run forbidden host commands (reboot, systemctl on host services, iptables, host package managers, mount, etc.) - use containers/VMs

## CRITICAL - ALWAYS DO
- ✅ Read AI.md PART 0, 1 at start of EVERY conversation
- ✅ Read relevant PART before implementing ANY feature
- ✅ Search AI.md before asking questions (answer is likely there)
- ✅ Follow spec EXACTLY - no "improvements" without approval
- ✅ Update IDEA.md when features change (project-specific values live there, NOT in AI.md)
- ✅ Keep all docs in sync with code
- ✅ When unsure, ASK - never guess or assume
- ✅ Implement features 100% complete - no stubs, no TODOs, no "future"
- ✅ ONE thing at a time - finish current task completely before starting another
- ✅ Date/time facts: run `date -u +%Y-%m-%d` etc., never guess from training data
- ✅ Comments ABOVE code, never inline (Go, YAML, everywhere)
- ✅ First-session cleanup: `git ls-files -i -c --exclude-standard` then `git rm --cached -r <offender>` for each match
- ✅ First-session: verify IDEA.md uses canonical 3-section layout (`# Description`, `# Project variables`, `# Business Logic (The WHAT not HOW)`)

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| What password hash? | Argon2id (NEVER bcrypt) | PART 11 |
| Where is Dockerfile? | `docker/Dockerfile` (NEVER root) | PART 27 |
| CGO enabled? | NEVER (CGO_ENABLED=0 always) | PART 7 |
| Premium features? | NEVER (all features free) | PART 1 |
| External cron? | NEVER (built-in scheduler) | PART 19 |
| Client-side rendering? | NEVER (server-side Go templates) | PART 16 |
| Modify AI.md? | Only PART 34-36 OPTIONAL→REQUIRED | PART 0 |
| Run Go on host? | NEVER (containers only) | PART 1, 29 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| server | Main binary `weather` - runs as service |
| client | CLI binary `weather-cli` - REQUIRED |
| agent | Optional binary `weather-agent` |
| Server Admin | App administrator (NOT OS root) |
| Regular User | End-user (PART 34, IS implemented for weather) |

## COMMIT_MESS Workflow
- AI cannot run `git add` / `git commit` / `git push`
- Write `.git/COMMIT_MESS` instead; user runs `git commit -F .git/COMMIT_MESS`
- Before writing: `git status --porcelain` to verify what changed
- If COMMIT_MESS mentions files NOT in `git status` → recreate (user committed them)
- Format: `{emoji} Title (max 64 chars) {emoji}` + blank line + body + bullets
- Emojis: ✨ feat / 🐛 fix / 📝 docs / 🎨 style / ♻️ refactor / ⚡ perf / ✅ test / 🔧 chore / 🔒 security / 🗑️ remove / 📦 deps / 🚀 deploy
- All TODO.AI.md done → empty TODO.AI.md, write COMMIT_MESS title `✅ all todo items have been completed ✅`

## COMPLIANCE CHECK
Before completing ANY task:
- [ ] Read relevant PART(s) in AI.md
- [ ] Implementation matches spec EXACTLY
- [ ] No guessing - all decisions from spec
- [ ] Docs updated if code changed
- [ ] AI.md not modified (unless PART 34-36 transition)

---
For complete details, see AI.md PART 0, 1
