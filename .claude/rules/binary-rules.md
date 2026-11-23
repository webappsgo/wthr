# Binary, CLI & Agent Rules (PART 7, 8, 33)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER Change binary names, flags, exit behavior, service interfaces, or agent behavior by assumption.
- NEVER Run a server binary in a way that bypasses the prescribed config, data, or privilege boundaries.
- NEVER Add undocumented CLI options or environment-driven behavior.

## CRITICAL - ALWAYS DO

- ALWAYS Implement the exact binary, CLI, maintenance, and agent contracts in these PARTS.
- ALWAYS Keep help output, errors, exit codes, and documented flags synchronized with behavior.
- ALWAYS Use the prescribed startup, shutdown, and process-safety behavior.

## Key Rules Summary

Binary requirements, server CLI, and client/agent behavior are defined by PARTS 7, 8, and 33.

For complete details and exact behavior, read AI.md PART 7, 8, 33. AI.md is authoritative; this file is only a topic-scoped quick reference.
