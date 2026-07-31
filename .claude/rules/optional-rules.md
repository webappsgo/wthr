# Optional Features Rules (PART 34-36)

⚠️ **These rules are NON-NEGOTIABLE ONCE IMPLEMENTED. Violations are bugs.** ⚠️

## Activation status for this project (wthr)

There is no `SPEC.md` in this repo; activation is declared in `IDEA.md` instead.

- **PART 34 (Multi-User): ACTIVE / REQUIRED** — IDEA.md explicitly states
  "User account rules (Multi-User — PART 34 REQUIRED)". Regular user accounts
  (saved locations, alert subscriptions) use password auth, not passkey-only.
- **PART 35 (Organizations): DORMANT** — IDEA.md: "Organization/multi-tenant
  features (PART 35 not adopted)". Do not build org/team features unless
  IDEA.md is updated to require them.
- **PART 36 (Custom Domains): DORMANT** — IDEA.md: "Custom domain features
  (PART 36 not adopted)". Do not build custom-domain support unless IDEA.md
  is updated to require it.

Because PART 34 is active, all "NEVER/ALWAYS" rules below for Multi-User
apply now. PART 35/36 rules are reference-only until adopted.

## CRITICAL - NEVER DO

- NEVER let Server Admin (PART 17) view a user's full email, password,
  recovery keys, or 2FA secret — only masked email, username, status,
  last login are visible to admin.
- NEVER let Server Admin set/change a user's password — only the user can,
  via invite/reset link.
- NEVER reveal whether a username/email exists on login, registration, or
  password-reset failure — always generic errors
  ("Invalid credentials...", "If an account exists, instructions have
  been sent.").
- NEVER store recovery keys, TOTP secrets, or passwords in plaintext —
  recovery keys are SHA-256 hashed and shown to the user exactly once.
- NEVER allow a blocklisted username (admin, root, system, api, www, etc. —
  full list in AI.md PART 34) or one colliding with a reserved route path.
- NEVER allow user-uploaded or externally-linked avatars to be served as
  SVG (active content risk) — raster only unless sanitized/rasterized at
  ingest.
- NEVER expose a private user/org profile via API, search, listings, or
  direct URL to non-owners — return 404, not 403 (don't leak existence).
  Exception: the user themselves, server admins (via admin panel only),
  and same-org members (username only).
- NEVER let registration mode `disabled`/`invite`/`admin_only` leave
  `/server/auth/register` reachable — must 404 when public self-registration
  isn't allowed for the active mode.
- Org (PART 35, if ever adopted): NEVER let `server.orgs.creation.mode` be
  confused with per-org `visibility` — creation mode is server-level policy,
  visibility is per-org.
- Custom Domains (PART 36, if ever adopted): NEVER activate a custom domain
  without TXT-record ownership verification; NEVER skip automatic SSL
  (Let's Encrypt HTTP-01/TLS-ALPN-01/DNS-01) when `require_ssl: true`.

## CRITICAL - ALWAYS DO

- ALWAYS treat Server Admin (PART 17) and Regular User (PART 34) as
  separate account types with separate tables (`admins` vs `users`) and
  separate route namespaces (`/server/{admin_path}/*` vs `/users/*`).
- ALWAYS require the user to re-enter their password before starting
  TOTP or passkey setup, and require the "I saved my recovery keys"
  checkbox before activating 2FA/passkey.
- ALWAYS log admin actions on user accounts (password-reset trigger,
  2FA disable, suspend) with a required audit reason.
- ALWAYS enforce case-insensitive uniqueness for username, email, and
  (if PART 35) org slug — store lowercase, compare lowercase.
- ALWAYS respect the four registration modes consistently: `open`
  (self-register + invite + admin-create), `invite` (admin invite only),
  `admin_only` (admin creates + activation link only), `disabled`
  (no new accounts, existing users can still log in).
- ALWAYS validate emails/usernames per the RFC-5321/5322-based rules in
  AI.md PART 34 (length limits, allowed chars, no leading/trailing/
  consecutive dots).
- ALWAYS honor Account Email (security-only) vs Notification Email
  (general) as separate fields; account-email changes require current
  password + 2FA if enabled.
- Org (PART 35, if adopted): ALWAYS require PART 34 (multi-user) to be
  implemented first; ALWAYS give orgs Owner/Admin/Member roles with the
  documented permission boundaries.
- Custom Domains (PART 36, if adopted): ALWAYS scope domains to `user` or
  `org` ownership with per-owner limits (`max_domains_per_user`,
  `max_domains_per_org`); ALWAYS block reserved/government/education
  patterns from `blocked_patterns`/`reserved`.

## Key rules summary

### Multi-User (PART 34) — ACTIVE for this project
- Two account types: Server Admin (always required, PART 17) vs Regular
  User (optional, this PART). Admin auth is passkey-only; regular users use
  password auth (per IDEA.md decision — low-stakes feature).
- Registration modes: `open` (default), `invite`, `admin_only`, `disabled` —
  set via `users.registration.mode` in config.
- Auth methods: password, TOTP 2FA, WebAuthn passkeys, OIDC/LDAP/SAML.
- Recovery keys: 10 keys, format `{8-hex}-{4-hex}`, generated once when
  2FA/passkey enabled, SHA-256 hashed, single-use, case-insensitive.
- Account recovery requires knowing username OR email + password; losing
  both means the account is unrecoverable by design (no PII escrow).
- Server Admin's only recovery path: `{project_name} --maintenance setup`
  (console/SSH access required, no prior credentials needed).
- Login identifier auto-detected: numeric = user ID, contains `@` = email,
  else = username.
- Profile visibility: `public` (default) or `private`; private = 404 to
  non-owners/non-admins/non-org-members.
- Avatar: gravatar (default) / upload (max 2MB, 64-1024px, raster only) /
  external URL (raster only).

### Organizations (PART 35) — DORMANT for this project
- Requires PART 34 first. Canonical internal term is "organization"; UI
  copy may say team/workspace/group.
- Org creation modes: `open` (default), `invite`, `admin_only`, `disabled`
  — set via `server.orgs.creation.mode` (server-level, distinct from
  per-org `visibility`).
- Roles: Owner (full control incl. delete/transfer) > Admin (manage
  members/settings/tokens) > Member (view/use resources).
- Org-scoped user visibility: members can see "basic info" of otherwise-
  private co-members within the shared org context only.

### Custom Domains (PART 36) — DORMANT for this project
- Requires public-facing, user/org-branded content to justify; subdomains
  are the simpler default — add custom domains only on demand.
- Ownership: `user` or `org`, each with configurable domain limits.
- Flow: user adds domain -> configures DNS -> triggers TXT-record
  verification -> automatic SSL issuance (HTTP-01/TLS-ALPN-01, or DNS-01
  for manual/wildcard cases).
- Config toggle: `server.features.custom_domains.enabled` (default false)
  plus per-user/per-org limits, apex/subdomain/wildcard allow-flags,
  reserved domains, and blocked-pattern regexes (gov/mil/edu TLDs blocked
  by default).

For complete details, see AI.md PART 34-36
