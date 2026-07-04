# Frontend Rules (PART 16, 17)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Use React, Vue, Angular, or any SPA framework → server-side Go templates only
- Require JavaScript for core features → progressive enhancement only
- Hardcode colors → use CSS custom properties (variables)
- Use light mode as default → dark mode is default
- Let long strings break mobile → use `word-break: break-word` CSS
- Put admin routes outside `/server/{admin_path}/` scope
- Skip CSRF protection on any mutating web form

## CRITICAL - ALWAYS DO

- Server-side rendering: Go `html/template` only
- Mobile-first responsive CSS
- Dark mode default; support dark / light / auto via `prefers-color-scheme`
- CSS custom properties for all colors and spacing
- All features work without JavaScript (JS is progressive enhancement)
- WCAG 2.1 AA accessibility
- 44×44px minimum touch targets
- Templates in `src/server/template/`

## Template Organization

```
src/server/template/
├── admin/         # Admin panel templates
├── auth/          # Login, register, 2FA templates
├── page/          # Main page templates
│   ├── dashboard.tmpl
│   ├── add_location.tmpl
│   └── ...
├── partials/      # Reusable template fragments
└── base.tmpl      # Base layout
```

## Admin Panel (PART 17)

All admin at `/server/{admin_path}/` (default: `/server/admin/`):

```
GET  /server/admin/              # Admin dashboard
GET  /server/admin/users         # User management
GET  /server/admin/config        # Server config
GET  /server/admin/logs          # Log viewer
GET  /server/admin/email         # Email templates editor
GET  /server/admin/notifications # Notification settings
GET  /server/admin/passkeys      # Passkey management
GET  /server/admin/metrics       # Metrics dashboard
GET  /server/admin/backup        # Backup & restore
GET  /server/admin/update        # Update management
GET  /server/admin/security      # Security settings
```

Admin API at `/api/v1/server/{admin_path}/`:

```
GET    /api/v1/server/admin/users
POST   /api/v1/server/admin/users
DELETE /api/v1/server/admin/users/:id
GET    /api/v1/server/admin/config
PUT    /api/v1/server/admin/config
GET    /api/v1/server/admin/logs
...
```

## Reference

For complete details, see AI.md PART 16, 17
