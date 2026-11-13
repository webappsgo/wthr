# TODO

- Admin panel pages (admin/*.tmpl) are not covered by the browser matrix; the admin login is, the signed-in admin pages need a setup-token flow in tests/e2e/layout.mjs.
- Setup wizard pages (page/setup_*.tmpl) still use their own chromeless body instead of the shared header, nav and footer.
- Remaining hardcoded English in templates: page/help.tmpl prose, admin/admin.tmpl, admin_email.tmpl provider labels, page/earthquake.tmpl CDI/MMI labels, unit suffixes (mph, km/h) on weather pages.
