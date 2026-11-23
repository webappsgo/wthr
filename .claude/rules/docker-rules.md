# Docker & Container Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER Place Docker or Compose files outside the required directories or use unsafe defaults.
- NEVER Require committed `.env` files or host paths that violate runtime-data boundaries.
- NEVER Use broad container cleanup or privileged runtime settings without a spec requirement.

## CRITICAL - ALWAYS DO

- ALWAYS Follow the prescribed image, build, runtime user, mount, network, and default configuration.
- ALWAYS Keep Docker build/test execution isolated and container data out of the repository.
- ALWAYS Keep image and Compose documentation aligned with actual files.

## Key Rules Summary

Docker build and runtime requirements are defined by PART 27.

For complete details and exact behavior, read AI.md PART 27. AI.md is authoritative; this file is only a topic-scoped quick reference.
