# Project SPEC

Project: Weather
Role: Efficient loader for AI.md

⚠️ **THIS FILE IS AUTO-LOADED EVERY CONVERSATION. FOLLOW IT EXACTLY.** ⚠️

Purpose:
- This file is a short loader for the most important rules
- `AI.md` is the full source of truth
- For complete details, read the referenced PARTs in `AI.md`

## FIRST TURN - MANDATORY

On EVERY new conversation or after "context compacted" message:
1. **READ** the relevant `.claude/rules/*.md` for your current task
2. **NEVER** assume or guess - verify against AI.md before implementing

## Asking Questions

- **Default to continuing work** — do not stop just to ask whether you should continue
- **Never guess** — if the answer cannot be determined from `AI.md`, `IDEA.md`, the codebase, or repo state and the missing information materially changes behavior, scope, or safety, ASK the user
- **Do NOT ask for permission to keep going** — continue until the current task is complete, blocked by a real decision, or the user explicitly asks to pause
- **Question mark = question** — when user ends with `?`, answer/clarify, don't execute
- **Use AskUserQuestion wizard** — one question at a time with options + "Other" for custom input

**Ask only when at least one of these is true:**
1. A required business/product decision is missing
2. Two or more reasonable implementations would produce materially different behavior
3. The action is destructive, irreversible, or impacts production/user data
4. The spec explicitly says to ask or confirm
5. The user explicitly requested a plan, pause, or checkpoint before execution

## Before ANY Code Change

1. Have I read the relevant PART in AI.md? (If no → read it)
2. Does this follow the spec EXACTLY? (If unsure → check spec)
3. Am I guessing or do I KNOW from the spec? (If guessing → read spec)
4. Would this pass the compliance checklist? (AI.md FINAL section)

**WHEN IN DOUBT: READ THE SPEC. DO NOT GUESS.**

## Binary Terminology
- **server** = `wthr` (main binary, runs as service)
- **client** = `wthr-cli` (REQUIRED companion, CLI/TUI/GUI)
- **agent** = `wthr-agent` (optional, runs on remote machines)

## Key Placeholders
- `{project_name}` = wthr
- `{project_org}` = casapps
- `{admin_path}` = admin

## Account Types (CRITICAL)
- **Server Admin** = manages the app (NOT a privileged OS user)
- **Primary Admin** = first admin, cannot be deleted
- **Regular User** = end-user (PART 34, REQUIRED for this project)
- Server Admins ≠ Regular Users (separate DB tables: server.db vs users.db)

## NEVER Do (Top 19) - VIOLATIONS ARE BUGS
1. Use bcrypt → Use Argon2id
2. Put Dockerfile in root → `docker/Dockerfile`
3. Use CGO → CGO_ENABLED=0 always
4. Hardcode dev values → Detect at runtime
5. Use external cron → Internal scheduler (PART 19)
6. Store passwords plaintext → Argon2id (tokens use SHA-256)
7. Create premium tiers → All features free, no paywalls
8. Use Makefile in CI/CD → Explicit commands only
9. Guess or assume values → Run the command or read spec
10. Skip platforms → Build all 8 (linux/darwin/windows × amd64/arm64 + freebsd × amd64/arm64)
11. Client-side rendering (React/Vue) → Server-side Go templates
12. Require JavaScript for core features → Progressive enhancement only
13. Let long strings break mobile → Use word-break CSS
14. Skip validation → Server validates EVERYTHING
15. Implement without reading spec → Read relevant PART first
16. Modify AI.md content → READ-ONLY
17. Edit `## Project variables` in IDEA.md without confirming with user
18. Read an image larger than 1000×1000 directly into context → Resize first
19. Use a non-conforming IDEA.md without migration → Migrate first

## ALWAYS Do - NON-NEGOTIABLE
1. Read AI.md before implementing ANY feature
2. Server-side processing (server does the work, client displays)
3. Mobile-first responsive CSS
4. All features work without JavaScript
5. Tor hidden service support (auto-enabled if Tor found)
6. Built-in scheduler, GeoIP, metrics, email, backup, update
7. Full admin panel with ALL settings
8. Client binary for ALL projects (wthr-cli)
9. Commit often — small, focused commits

## File Locations
- Config: `/etc/casapps/wthr/server.yml` (root) or `~/.config/casapps/wthr/server.yml` (user)
- Data: `/var/lib/casapps/wthr/` (root) or `~/.local/share/casapps/wthr/` (user)
- Logs: `/var/log/casapps/wthr/` (root) or `~/.local/log/casapps/wthr/` (user)
- Source: `src/`
- Docker: `docker/`

## Where to Find Details
- AI behavior: `.claude/rules/ai-rules.md` (PART 0, 1)
- Project structure: `.claude/rules/project-rules.md` (PART 2, 3, 4)
- Frontend/WebUI: `.claude/rules/frontend-rules.md` (PART 16, 17)
- Full spec: `AI.md` (~55k lines) ← **SOURCE OF TRUTH**

## Current Project State
- Last read AI.md: 2026-05-16
- Current task: Bootstrap complete
- Relevant PARTs: 0-6, 34 (multi-user REQUIRED)
