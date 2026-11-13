# TODO

- Earthquake pages load Leaflet 1.9.4 from unpkg.com without SRI (earthquake.tmpl, earthquake_detail.tmpl); embed it under static/ or add integrity hashes, then drop the unpkg CSP extras in src/server/middleware/security_headers.go.
- Notification dropdown strings built in app.js (Notifications module) are hardcoded English; move to /locales/{lang}.json keys.
- Handlers set English page titles (e.g. "Severe Weather Alerts" in handler/severe_weather.go); translate them.
- Matrix coverage: add auth pages, user pages and the admin login to tests/e2e/layout.mjs.
