# Privilege Escalation & Service Rules (PART 24, 25)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER Run permanently as a privileged account unless IDEA.md explicitly approves it.
- NEVER Use reserved service IDs, a login shell, or destructive uninstall behavior outside the spec.
- NEVER Assume service-manager availability or privilege-escalation capability.

## CRITICAL - ALWAYS DO

- ALWAYS Follow the specified privilege drop, account allocation, service lifecycle, and platform support.
- ALWAYS Check whether the process is already privileged before prompting for escalation.
- ALWAYS Preserve application data during service removal unless the spec explicitly says otherwise.

## Key Rules Summary

Privilege escalation and OS service support are defined by PARTS 24–25.

For complete details and exact behavior, read AI.md PART 24, 25. AI.md is authoritative; this file is only a topic-scoped quick reference.
