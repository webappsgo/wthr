# Frontend & Admin UI Rules (PART 16, 17)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER Add inline executable JavaScript, inline event handlers, inline styles, or page-specific shell overrides.
- NEVER Create unlabeled cards, inaccessible forms, duplicate page titles, or inconsistent shared navigation.
- NEVER Expose admin pages through public navigation or use public auth/session state for the isolated admin panel.

## CRITICAL - ALWAYS DO

- ALWAYS Use the shared page shell, accessible landmarks, heading hierarchy, section cards, forms, CSRF, and static asset rules.
- ALWAYS Keep public and admin chrome consistent within their respective shell rules.
- ALWAYS Keep all user-visible strings translated and update frontend tests for changed behavior.

## Key Rules Summary

Frontend shell, accessibility, shared assets, and admin panel behavior are defined by PARTS 16–17.

For complete details and exact behavior, read AI.md PART 16, 17. AI.md is authoritative; this file is only a topic-scoped quick reference.
