# Backend, Database & Security Rules (PART 9, 10, 11, 32)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER Leak credentials, tokens, private user data, SQL details, stack traces, or internal paths.
- NEVER Bypass the specified database, cluster consistency, auth, cryptography, rate-limit, or audit rules.
- NEVER Silently discard errors or weaken security controls to simplify a flow.

## CRITICAL - ALWAYS DO

- ALWAYS Use the specified error model, caching, database lifecycle, security controls, and audit behavior.
- ALWAYS Validate and authorize user-controlled inputs before storage, network, or command use.
- ALWAYS Keep overlay network behavior bounded and consistent with the prescribed health and failure model.

## Key Rules Summary

Backend behavior, database/cluster, security, and Tor/I2P requirements are defined by PARTS 9–11 and 32.

For complete details and exact behavior, read AI.md PART 9, 10, 11, 32. AI.md is authoritative; this file is only a topic-scoped quick reference.
