# Web Frontend & Admin Panel Rules (PART 16, 17)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER ship a frontend that breaks without JavaScript — JS enhances, it does not enable; core nav/forms/CRUD MUST work with JS disabled
- NEVER use `alert()`, `confirm()`, or `prompt()` — use native `<dialog>` styled with CSS
- NEVER use inline `onclick`/inline JS handlers — CSP blocks them; all JS lives in `static/js/app.js`, bound via `data-action` delegation
- NEVER use inline `<style>`/`style="..."` — all CSS in `common.css`/`public.css`/`admin.css`/`components.css`
- NEVER use a JS framework (React/Vue/Alpine/jQuery), bundler (webpack/vite/rollup), or transpiler (TypeScript/Babel) — plain browser-native JS only, ONE file
- NEVER use desktop-first CSS (`max-width` media queries) — mobile-first only (`min-width`)
- NEVER put the theme class on `<body>` — it goes on `<html>` (`theme-dark`/`theme-light`/`theme-auto`)
- NEVER create layout-specific theme files (`admin-dark.css`, `public-light.css`) — one shared theme system for public + admin + Swagger + GraphiQL + CLI + TUI + GUI
- NEVER link to `/server/{admin_path}` from ANY public route, nav, or footer — admin panel must not be discoverable
- NEVER put server management routes directly under `/server/{admin_path}/` — everything except the admin's own account goes under `/server/{admin_path}/config/*`
- NEVER let long strings (IPv6, .onion, tokens, hashes, UUIDs) overflow their container — always `word-break: break-all` / `overflow-wrap: break-word` or horizontal scroll
- NEVER leave a list/table/view with blank space when empty — always a proper empty state (icon, title, message, action)
- NEVER use `position: fixed`/`sticky` on header/nav/footer — only transient overlays (mobile nav panel, toasts, cookie banner) may be fixed
- NEVER pass user-controlled content through `template.HTML` unless it went through an allow-list sanitizer first
- NEVER store session tokens in localStorage/IndexedDB — session lives in `HttpOnly`+`Secure`+`SameSite` cookie only
- NEVER bypass admin auth for testing/dev convenience — `--debug` adds verbosity only, never bypasses auth
- NEVER give additional Server Admins weaker security than the Primary Admin (password rules, TOTP, passkeys, session timeout, audit logging all apply equally)
- NEVER use `!important` in CSS except print styles

## CRITICAL - ALWAYS DO

- ALWAYS make the frontend fully functional in-browser (not API-only), mobile-responsive, WCAG 2.1 AA, PWA-installable
- ALWAYS detect client type (browser/CLI/Accept header) and serve HTML to browsers, text to CLI/curl, JSON on request — same route, `detectClientType()`
- ALWAYS support CRUD via HTML forms (browsers), JSON (API), and form-encoded/text (CLI) on frontend routes
- ALWAYS use HTML5 → CSS → JS priority order; every line of JS must be justified (details/summary, dialog, :focus-within, checkbox-hack cover most "interactive" needs)
- ALWAYS use Go `html/template` (`.tmpl`) for all HTML — layouts (`public.tmpl`, `admin.tmpl`) + partials (header/nav/footer) + pages
- ALWAYS give every page header + nav (top) + footer (bottom) via shared partials — no page defines its own
- ALWAYS style error pages (400/401/403/404/500/502/503) with the site theme — never a bare/unstyled error page
- ALWAYS default to dark theme when no preference is set; server renders the `theme-*` class from cookie/DB (zero-JS correct-theme-on-load); `theme-auto` is pure CSS `prefers-color-scheme`
- ALWAYS keep WCAG AA contrast (4.5:1 normal text, 3:1 large text) in both light and dark themes
- ALWAYS show visible "Copied!" feedback (checkmark + `aria-live="polite"`) on every copy button
- ALWAYS disable the submit button immediately on click, show a loading label ("Saving...", "Deleting...", etc.), re-enable on success or error
- ALWAYS use toast for non-blocking confirmations ("Saved", "Copied") and modal for decisions/input/destructive confirmations — never interchange them
- ALWAYS provide a server-rendered flash-message fallback (POST-redirect-GET) for non-AJAX form submits — toasts are the JS enhancement layered on top, never the only feedback channel
- ALWAYS give forms inline, accessible validation: HTML5 constraints first, `:user-invalid` for styling, `aria-describedby`/`role="alert"` for errors, trim whitespace (but reject — don't trim — password whitespace)
- ALWAYS normalize URLs (strip trailing slash except root, 301 redirect) via middleware applied first in the chain
- ALWAYS keep the admin panel isolated: separate session, separate auth (`admins` table, not `users` table), configurable `server.admin_path` (default `admin`), reachable only by typing the URL directly
- ALWAYS put admin's own account under `/server/{admin_path}/{admin_username}/*` (profile, preferences, notifications) and all server management under `/server/{admin_path}/config/*`
- ALWAYS require the admin panel to exist for every project, be fully usable before the setup wizard runs (sane defaults, one-time setup token shown once at startup), and support TOTP + Passkey/WebAuthn MFA for every Server Admin
- ALWAYS validate new admin routes/paths against reserved names and existing routes before accepting them

## Key Rules Summary

**Layout & breakpoints:** mobile base (<768px) → `min-width:768px` tablet → `min-width:1024px` desktop → optional `min-width:1280px`. Container: 100% width/1rem padding on mobile, 90%/max-width:1400px on tablet+. Touch targets ≥44x44px. Footer always at bottom via flex `min-height:100vh` + `main{flex:1}`, never floating.

**Theme system:** one palette defined once (`src/common/theme/colors.go`) drives Web CSS, Swagger, GraphiQL, CLI, TUI, GUI. Three themes: dark (default), light, auto. Preference source: `theme` cookie (guests) → `user_preferences.theme`/`admin_preferences.theme` (logged-in) → fallback dark.

**Tech stack:** Go `html/template` HTML, vanilla JS (one `static/js/app.js` file), CSS-first styling, no inline CSS/JS, no frameworks/bundlers/npm for frontend.

**Standard `/server/*` pages:** about, privacy, contact, help, terms, healthz — all themed, all content sourced from IDEA.md (never generic placeholders), all with API JSON equivalents under `/api/{api_version}/server/*`.

**Admin panel layout:** header (logo/search/status/admin name/logout) + collapsible sidebar (Dashboard, Server, Security, Network, Users, Cluster, Help) + main content with breadcrumbs + compact footer. Login at `/server/{admin_path}`, session cookie 30 days default (90 with Remember Me), CSRF on all forms, optional TOTP MFA.

**Vanity URLs** (optional, PART 34/35): `/{username}`, `/{org_name}` — lowest route priority, registered last, reserved-name list blocks system paths, username/org share one namespace.

For complete details, see AI.md PART 16, 17
