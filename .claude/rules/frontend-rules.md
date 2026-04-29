# Frontend Rules (PART 16, 17)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Client-side rendering (React, Vue, Angular, Svelte, Solid, etc.)
- ❌ Single-page-app routing
- ❌ Require JavaScript for core functionality (login, registration, password reset, account settings, all admin pages, all user pages, all data submission)
- ❌ Put business logic, validation, or formatting in JavaScript
- ❌ Use JavaScript `alert()` / `confirm()` / `prompt()` — use toast notifications + server-side modals
- ❌ Inline CSS or JavaScript in templates — use external `.css` / `.js` files
- ❌ Place CSS files outside `src/server/template/static/css/` (or embedded equivalent)
- ❌ Skip mobile-first CSS — design for mobile FIRST, expand at breakpoints
- ❌ Let long strings (IPv6, .onion, tokens, hashes, UUIDs, Base64) break mobile layout — apply `word-break: break-all`
- ❌ Use generic placeholder content ("Your application name", "Feature 1", "Coming soon") on `/server/about`, `/server/help`, etc.
- ❌ Source `/server/about` and `/server/help` content from anywhere except IDEA.md
- ❌ Skip CSRF tokens on cookie-authenticated state-changing forms
- ❌ Apply CSRF to: Bearer/API-token requests, public endpoints, OAuth callbacks, webhooks (these are exempt)
- ❌ Hardcode `lang="en"` in `<html>` — use `lang="{{.Lang}}" dir="{{.Dir}}"` (PART 31)
- ❌ Skip Server Admin's full settings UI — every config option must be editable in admin panel (PART 17)
- ❌ Skip the `/server/about`, `/server/help`, `/server/privacy`, `/server/terms`, `/server/contact`, `/server/setup` (first-run only), `/server/security-report`, `/server/pubkey.asc` pages
- ❌ Use a default-allow CORS — explicit allow-list (PART 16 → CORS)
- ❌ Skip touch targets <44×44px on mobile
- ❌ Skip WCAG 2.1 AA compliance

## CRITICAL - ALWAYS DO
- ✅ Server-side rendering with Go `html/template`
- ✅ Progressive enhancement: page works without JS; JS only adds polish (theme toggle, copy-to-clipboard, form feedback)
- ✅ Mobile-first responsive CSS (breakpoints: base = mobile, `@media (min-width: 768px)` = tablet+, `@media (min-width: 1024px)` = desktop+)
- ✅ All long strings: `word-break: break-all; overflow-wrap: break-word; font-family: monospace;`
- ✅ CSRF tokens via `csrf_token_secret` HMAC; render in templates as `<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">`
- ✅ CORS allow-list resolution order: explicit `server.cors.allowed_origins` → `DOMAIN` env entries → reverse-proxy detected hosts. Credentials-aware echo (return only the requesting origin if it's allowed)
- ✅ CSP `connect-src`, `frame-ancestors`, `form-action` use `{learned_origins}` (the union of DOMAIN + reverse-proxy hosts)
- ✅ Sitemap (`/sitemap.xml`) auto-generated from registered routes
- ✅ Site verification meta tags (Google, Bing, etc.) configurable via admin panel
- ✅ Branding/SEO config in `server.branding.{title,tagline,description}` and `server.seo.{keywords[],...}`
- ✅ `/server/about` content from IDEA.md → `name`, `tagline`, `description`, `features`, `links`
- ✅ `/server/help` content from IDEA.md → real endpoints, real `curl` examples, real FAQ
- ✅ `/server/privacy` content from `server.privacy.*` config
- ✅ `/server/terms` content customizable, default template provided
- ✅ `/server/contact` recipient + webhooks from `server.contact.general.*` (falls back to `server.contact.admin.*`)
- ✅ Theme toggle: client-side JS for instant feedback; preference saved to `localStorage` (anonymous) or DB (logged-in user, requires PART 34)
- ✅ Toast notifications for transient feedback (no `alert()`)
- ✅ Server-side modals via HTMX-style `hx-target` or full-page reload — no SPA-style routing
- ✅ Touch targets: minimum 44×44 CSS pixels
- ✅ ALL admin settings have a UI page in `/admin/...` — no SSH/CLI required for any setting
- ✅ Live reload of config — changes apply immediately, except port/address (which warn restart-required)

## SERVER VS CLIENT (where logic lives)
| Task | Where | Why |
|------|-------|-----|
| Data validation | SERVER | Authoritative |
| HTML rendering | SERVER | Works without JS |
| Business logic | SERVER | Security, consistency |
| Formatting | SERVER | Consistent output |
| Theme toggle | Client JS | Instant UX |
| Copy to clipboard | Client JS | Browser API only |
| Form inline feedback | Client JS | UX polish only |

## REQUIRED PAGES
| Path | Source |
|------|--------|
| `/` | Project-specific (IDEA.md) |
| `/server/about` | IDEA.md → name, tagline, description, features |
| `/server/help` | IDEA.md → endpoints, examples, FAQ |
| `/server/privacy` | `server.privacy.*` |
| `/server/terms` | Configurable template |
| `/server/contact` | `server.contact.general.*` (fallback `admin.*`) |
| `/server/setup` | First-run only |
| `/server/security-report` | PGP-encrypted public summary |
| `/server/pubkey.asc` | Server's PGP public key |
| `/{admin_path}/...` | Admin panel (full coverage) |
| `/healthz` | Public status (HTML/JSON/text per Accept) |

## ADMIN PANEL (PART 17 summary)
- Full coverage of every `server.yml` setting
- Server Admin login at `/{admin_path}/login` (separate from user login)
- TOTP / Passkey / WebAuthn 2FA with recovery keys
- Scoped Agents API for per-agent tokens (`adm_agt_*`)
- Allowlist / Blocklist / GeoIP UIs
- Backup / Restore / Update / Maintenance Mode toggles
- Audit log viewer

---
For complete details, see AI.md PART 16, 17
