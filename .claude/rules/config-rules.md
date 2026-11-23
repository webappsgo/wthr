# Configuration & Runtime Modes (PART 5, 6, 12)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER Hardcode operator-editable settings, secrets, or environment-specific paths.
- NEVER Persist plaintext credentials or expose secret values through admin pages, logs, or APIs.
- NEVER Add a runtime mode, default, or configuration key that is not specified.

## CRITICAL - ALWAYS DO

- ALWAYS Use the configuration ownership, defaults, validation, reload behavior, and mode boundaries from these PARTS.
- ALWAYS Keep secrets in their prescribed secret store and expose only masked values where required.
- ALWAYS Document configuration changes in the paired project documentation.

## Key Rules Summary

Configuration, application modes, and server settings are defined by PARTS 5, 6, and 12.

For complete details and exact behavior, read AI.md PART 5, 6, 12. AI.md is authoritative; this file is only a topic-scoped quick reference.
