# Frontend Rules (PART 16, 17)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Client-side rendering (React, Vue, Angular, etc.)
- ❌ Require JavaScript for core functionality
- ❌ Client-side routing (SPA)
- ❌ Business logic in JavaScript
- ❌ Let long strings break mobile layout
- ❌ Desktop-first CSS (use mobile-first)
- ❌ Inline CSS or JavaScript
- ❌ JavaScript alerts (use toast notifications)
- ❌ Generic placeholder content in /server/about or /server/help pages
- ❌ "Your application name here" or "Feature 1, Feature 2" text
- ❌ Stub templates or "coming soon" pages
- ❌ Empty handlers or placeholder routes
- ❌ No CDN scripts — bundle all assets at build time (no `<script src="https://...">`)

## CRITICAL - ALWAYS DO
- ✅ Server-side rendering (Go templates)
- ✅ Progressive enhancement (works without JS)
- ✅ Mobile-first responsive CSS
- ✅ CSS `word-break: break-all` for long strings (IPv6, .onion, tokens)
- ✅ Full admin panel with ALL settings editable
- ✅ WCAG 2.1 AA accessibility
- ✅ Touch targets minimum 44×44px
- ✅ /server/about content from IDEA.md (name, tagline, description, features)
- ✅ /server/help content from IDEA.md (real endpoints, real examples)
- ✅ All pages fully functional — no "coming soon" or placeholder pages
- ✅ All routes implemented — no 501 Not Implemented responses
- ✅ Dark mode support (default dark, supports light/auto)

## PAGE CONTENT SOURCING
| Page | Content Source |
|------|----------------|
| /server/about | IDEA.md → name, tagline, description, features, links |
| /server/help | IDEA.md → real endpoints, real curl examples, real FAQ |
| /server/privacy | Config → `server.privacy.*` settings |
| /server/terms | Config → customizable, default template |
| /server/contact | Config → `server.contact.general.*` + `server.pages.contact.*` |

## ADMIN PANEL (PART 17)
The admin panel at `/{admin_path}/` (default `/admin/`) must have:
- Dashboard with system status, metrics, recent activity
- Settings page for ALL server config (writes back to server.yml)
- Admin user management (add/remove admins, passkey management)
- Log viewer with filtering
- Backup/restore interface
- Update management
- Scheduler status and manual trigger

## ADMIN AUTH FLOW
- Passkey (WebAuthn) — primary auth method
- Session token — 32 bytes crypto/rand, stored as SHA-256 in DB
- `adm:` prefix on session tokens for admin sessions
- Pending session token for passkey ceremony (stored in memory cache)

## SERVER VS CLIENT
| Task | Where | Why |
|------|-------|-----|
| Data validation | SERVER | Server is authoritative |
| HTML rendering | SERVER | Works without JS |
| Business logic | SERVER | Security, consistency |
| Theme toggle | Client JS | Instant UX feedback |
| Copy to clipboard | Client JS | Browser API required |
| Form feedback | Client JS | UX enhancement only |

## LONG STRINGS (REQUIRED CSS)
```css
.long-string, .ip-address, .onion-address, .api-token, .hash {
  word-break: break-all;
  overflow-wrap: break-word;
  font-family: monospace;
}
```

## BREAKPOINTS (mobile-first)
| Target | CSS |
|--------|-----|
| Mobile (base) | No media query |
| Tablet+ | `@media (min-width: 768px)` |
| Desktop+ | `@media (min-width: 1024px)` |

---
For complete details, see AI.md PART 16, 17
