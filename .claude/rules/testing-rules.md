# Testing Rules (PART 29, 30, 31)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Write tests that depend on external services → mock or skip
- Put test output in project tree → use `mktemp -d "/tmp/$(PROJECTORG)/..."` temp dir
- Skip tests when committing → `make test` must pass before every commit
- Use real databases in unit tests → use in-memory SQLite
- Skip i18n key parity check → run `scripts/i18n-validate.sh` before release
- Hardcode strings in templates → all user-visible strings via i18n keys
- Use i18n fallback silently → log missing keys as warnings

## CRITICAL - ALWAYS DO

- Table-driven tests with `t.Run()`
- Unit tests alongside code (`src/**/*_test.go`)
- Integration tests in `tests/unit/` and `tests/integration/`
- Coverage gate: 80% minimum (enforced in `make test`)
- In-memory SQLite for handler tests: `file:name?mode=memory&cache=shared`
- Test helpers in `tests/helpers/`
- i18n: all locales in `src/common/i18n/locales/{lang}.json`
- Reference locale: `en.json` (all keys must exist here)
- `scripts/i18n-validate.sh`: verify all locales have same keys as `en.json`

## Test Structure

```
tests/
├── unit/
│   └── handlers/        # Handler unit tests
├── integration/         # Full-stack integration tests
└── helpers/             # Shared test utilities
```

## i18n Structure (PART 31)

```
src/common/i18n/
└── locales/
    ├── en.json          # Reference locale (all keys required here)
    ├── es.json
    ├── fr.json
    └── ...
```

## Docs Structure (PART 30)

```
docs/
├── index.md
├── installation.md
├── configuration.md
├── api.md
└── ...
```

ReadTheDocs: `docs/` directory, `mkdocs.yml` config.

## Reference

For complete details, see AI.md PART 29, 30, 31
