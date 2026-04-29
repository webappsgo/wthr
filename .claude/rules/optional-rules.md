# Optional Rules (PART 34, 35, 36)

⚠️ **These PARTs are OPTIONAL — but once a project implements one, the entire PART becomes NON-NEGOTIABLE.** ⚠️

## STATUS FOR `weather`
- **PART 34 (Multi-User): IMPLEMENTED** — Regular User accounts/registration, vanity URLs, sessions, API tokens, 2FA, passkeys, profiles, avatars, notifications, saved locations, etc. ARE active in this project.
- **PART 35 (Organizations): NOT IMPLEMENTED** — multi-user orgs not in scope.
- **PART 36 (Custom Domains): NOT IMPLEMENTED** — user/org branded domains not in scope.

## CRITICAL - NEVER DO (because the inactive PARTs are fully absent)
- ❌ Add `organizations` table, org membership tables, org tokens, `/orgs/*` routes, or any org references — PART 35 is NOT implemented; unused features must be COMPLETELY ABSENT (PART 0 rule)
- ❌ Add `custom_domains` table, domain verification, user/org domain settings, custom-domain SSL — PART 36 is NOT implemented
- ❌ Skip the COMPLETE PART 34 surface — once partially implemented, everything in PART 34 is required
- ❌ Mix Server Admin and Regular User auth — separate tables (`srv_admins` vs `usr_users`), separate login routes (`/{admin_path}/login` vs `/auth/login`), separate session middleware
- ❌ Use bcrypt for user passwords — Argon2id only
- ❌ Issue user tokens without the `usr_` prefix; admin tokens without `adm_`; org tokens (when org exists) without `org_`; agent tokens with `usr_agt_` / `adm_agt_` / `org_agt_`
- ❌ Allow public registration if `users.registration.mode: invite` (default in this project per current TODO context)
- ❌ Skip email verification flow when SMTP is configured
- ❌ Allow user passwords with leading/trailing whitespace
- ❌ Send unencrypted password reset tokens — short-lived hashed tokens only
- ❌ Skip vanity URL availability check (`/users/{username}` must reject reserved + already-taken usernames)

## CRITICAL - ALWAYS DO (PART 34, since it IS implemented here)
- ✅ User table `usr_users` with: id, email (verified flag), username (unique, case-insensitive), display_name, password_hash (Argon2id), avatar_url, bio, location, website, role, status, created_at, updated_at, last_login_at, language, theme
- ✅ User session table `usr_user_sessions` (separate from admin)
- ✅ User API tokens table `usr_api_keys` with `usr_` prefix; max 5 per user; create-only plaintext return; SHA-256 hashed at rest
- ✅ User invites table `usr_user_invites` for invite-only registration; expiry options `1h|6h|24h|48h|7d`, default `24h`
- ✅ User 2FA: TOTP (`usr_totp_secrets`) + Passkeys (`usr_passkeys`) + recovery codes
- ✅ Login flow with passkey-as-second-factor: password verify → if user has passkeys, return `session_token` for pending session → user completes via challenge/verify → full session issued
- ✅ Vanity URLs: `/users/{username}` (public profile), `/users/settings` (own settings), `/users/security/passkeys` (passkey CRUD), `/users/avatar`, `/users/notifications`, `/users/locations`
- ✅ User avatar: HTTPS URL (validated), or upload (≤2MB), or Gravatar fallback
- ✅ Public profile masks email, respects privacy flag, only shows masked email when self-viewing
- ✅ Notifications: in-app (`usr_notifications`) + email (per-user opt) + browser push (per-user opt)
- ✅ Saved locations: per-user list with name + lat/lon + tz + alerts
- ✅ Join Cluster Flow (Technical, PART 34): when this user-side data joins a cluster, sync via the same DB → `server.yml` cache pattern
- ✅ Audit log records ALL user-side admin actions: `user.created`, `user.updated`, `user.password_changed`, `user.token_created`, `user.token_revoked`, `user.passkey_added`, `user.passkey_removed`, `user.deleted`

## ROUTE CANON (PART 34 → user-facing routes)
| Web | API | Purpose |
|-----|-----|---------|
| `/users/{username}` | `/api/v1/public/users/{username}` | Public profile |
| `/users/settings` | `/api/v1/users/settings` | Account / privacy / notifications / appearance |
| `/users/security/password` | `/api/v1/users/security/password` | Change password (current+new) |
| `/users/security/2fa` | `/api/v1/users/security/2fa` (status/setup/enable/disable/verify) | TOTP |
| `/users/security/recovery/regenerate` | `/api/v1/users/security/recovery/regenerate` | Regenerate recovery codes |
| `/users/security/passkeys` | `/api/v1/users/security/passkeys` (list/register/delete) | Passkey CRUD |
| `/users/avatar` | `/api/v1/users/avatar` (read/update/upload/reset) | Avatar |
| `/users/tokens` | `/api/v1/users/tokens` (list/create/revoke) | API tokens, max 5, `usr_` prefix |
| `/users/notifications` | `/api/v1/users/notifications` | In-app notifications |
| `/users/locations` | `/api/v1/users/locations` | Saved weather locations |
| `/auth/register` | `/api/v1/auth/register` | Public registration (invite-only mode rejects) |
| `/auth/invite/user/{token}` | `/api/v1/auth/invite/user/{token}` (validate/complete) | Invite acceptance |
| `/auth/login` | `/api/v1/auth/login` | Password login (returns `session_token` if passkey/2FA pending) |
| `/auth/passkey` | `/api/v1/auth/passkey/challenge` + `/verify` | Passkey login |
| `/auth/2fa` | `/api/v1/auth/2fa` | TOTP completion |
| `/auth/recovery/use` | `/api/v1/auth/recovery/use` | Recovery code login |
| `/auth/logout` | `/api/v1/auth/logout` | Invalidate session |
| `/auth/refresh` | `/api/v1/auth/refresh` | Rotate session |
| `/auth/password-reset` | `/api/v1/auth/password-reset/request` + `/reset` | Password reset |
| `/auth/verify-email` | `/api/v1/auth/verify-email` | Email verification |

## GRAPHQL PARITY
PART 34's REST surface MUST have GraphQL parity:
- `currentUser` / `updateUserProfile` / `userSettings` / `updateUserSettings`
- `currentUserAvatar` / `updateUserAvatar` / `uploadUserAvatar` / `resetUserAvatar`
- `userTokens` / `createUserToken` / `revokeUserToken`
- `currentUserTwoFactorStatus/Setup` / `enableUserTwoFactor` / `disableUserTwoFactor` / `verifyUserTwoFactor` / `regenerateUserRecoveryKeys`
- `currentUserPasskeys` / `beginUserPasskeyRegistration` / `finishUserPasskeyRegistration` / `deleteUserPasskey` / `beginUserPasskeyChallenge` / `finishUserPasskeyChallenge`
- `register` / `login` / `2faComplete` / `recoveryUse` / `logout` / `refresh` / `passwordReset*` / `verifyEmail`
- `validateUserInvite` / `completeUserInvite`
- `publicUserProfile(username)`

---
For complete details, see AI.md PART 34, 35, 36
