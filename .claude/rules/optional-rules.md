# Optional Features Rules (PART 34-36)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## STATUS FOR THIS PROJECT
| Feature | Status | Notes |
|---------|--------|-------|
| Multi-User (PART 34) | **REQUIRED** | Enabled — users.db, user accounts |
| Organizations (PART 35) | OPTIONAL | Not enabled for weather |
| Custom Domains (PART 36) | OPTIONAL | Not enabled for weather |

## CRITICAL - NEVER DO
- ❌ Mix Server Admin accounts and Regular User accounts in the same DB table
- ❌ Allow Regular Users to access admin panel endpoints
- ❌ Allow unauthenticated users to save locations (requires account)
- ❌ Store Regular User passwords with anything other than Argon2id
- ❌ Create premium/paid tiers — all user features free
- ❌ Gate any feature behind a subscription

## CRITICAL - ALWAYS DO (Multi-User — PART 34)
- ✅ Separate DB files: `server.db` (app/admin), `users.db` (regular users)
- ✅ Server Admins and Regular Users NEVER share DB tables
- ✅ Regular User registration: configurable (open/invite-only/disabled)
- ✅ User email verification before account activation
- ✅ User self-service: change password, change email, delete account
- ✅ Per-user saved locations (up to 10)
- ✅ Per-user alert subscriptions
- ✅ User session management (list sessions, revoke session)

## MULTI-USER DATA MODEL
| Entity | DB | Table |
|--------|----|-------|
| Server Admins | server.db | `admins` |
| Admin sessions | server.db | `admin_sessions` |
| Admin passkeys | server.db | `admin_passkeys` |
| Regular Users | users.db | `users` |
| User sessions | users.db | `user_sessions` |
| User locations | users.db | `user_locations` |
| User subscriptions | users.db | `user_subscriptions` |

## USER REGISTRATION MODES
| Mode | Config value | Behavior |
|------|-------------|---------|
| Open | `registration: open` | Anyone can register |
| Invite-only | `registration: invite` | Requires invite code |
| Disabled | `registration: disabled` | No new accounts |

---
For complete details, see AI.md PART 34, 35, 36
