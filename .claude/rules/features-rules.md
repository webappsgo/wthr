# Application Feature Rules (PART 18–23)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER Implement feature behavior or workflows from assumptions instead of the relevant feature PART.
- NEVER Expose third-party credentials, skip authorization, or add external services that the spec excludes.
- NEVER Leave feature documentation, admin controls, or user-facing translations stale after a feature change.

## CRITICAL - ALWAYS DO

- ALWAYS Implement each feature using its specified data lifecycle, permissions, defaults, and failure behavior.
- ALWAYS Keep feature settings editable through the prescribed interface and record sensitive actions as required.
- ALWAYS Update IDEA.md and user/developer documentation when feature behavior changes.

## Key Rules Summary

Email, notifications, scheduling, GeoIP, metrics, backup/restore, and updates are defined by PARTS 18–23.

For complete details and exact behavior, read AI.md PART 18–23. AI.md is authoritative; this file is only a topic-scoped quick reference.
