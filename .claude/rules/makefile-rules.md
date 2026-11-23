# Makefile & Local Development Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER Add targets, dependencies, or local developer workflows that conflict with the fixed target contract.
- NEVER Put generated binaries or test data in tracked source directories.
- NEVER Use Makefile targets in CI where the spec requires explicit commands.

## CRITICAL - ALWAYS DO

- ALWAYS Keep developer targets, variables, outputs, and cleanup behavior aligned with PART 26.
- ALWAYS Use project-scoped locations for build outputs and local runtime artifacts.
- ALWAYS Keep README development instructions synchronized with actual targets.

## Key Rules Summary

The Makefile and local development workflow are defined by PART 26; CI rules are in PART 28.

For complete details and exact behavior, read AI.md PART 26. AI.md is authoritative; this file is only a topic-scoped quick reference.
