# TODO.AI.md

Implementation work remaining for full alignment with `AI.md`.

- [ ] Complete Mixed Mode end to end: configured replica connections, trigger-based primary outbox capture, durable Valkey/Redis publication, consumer retries, schema-whitelisted row application, and consistent initial snapshot/bootstrap.
- [ ] Bring the configured MSSQL replica example into support: connect, create/verify the standard schema, capture/apply changes, and cover SQL Server placeholder, DDL, and upsert behavior.
- [ ] Reconcile Mixed Mode startup, shutdown, offline replica recovery, and schema-version behavior with the required database lifecycle.
- [ ] Review affected configuration, admin pages, translations, and documentation; update every surface that exposes or explains the completed replication behavior.
- [ ] Align the shared frontend with the CSS architecture: layer order and file layers, eliminate duplicate and legacy utility rules, meet logical-property/token/catalog rules, add the missing stylelint gate, and keep the public/admin stylesheet selection isolated.
- [ ] Review every `/server/**` HTML route in browser against the complete AI.md layout matrix, fixing route rendering, responsive layout, themes, RTL, keyboard behavior, and browser-console failures.
- [ ] Align `make test` with AI.md PART 26 and PART 29: include `govulncheck`, `shellcheck`, `shfmt -d`, coverage, and `tests/run_tests.sh`; include the CSS stylelint gate required by PART 16.
- [ ] Re-read the remaining applicable `AI.md` sections, address any additional project gaps, and keep template contradictions only in the parent `TODO.SERVER.md` inbox.
- [ ] Run and resolve the prescribed tests, linters, build, strict documentation build, CI checks, and final `main`-branch/diff review after implementation is integrated.
