# Testing Rules (PART 29, 30, 31)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Run `go test` directly on host — always via Docker
- ❌ Tests that depend on host state or network — use mocks/stubs
- ❌ Tests that write to real filesystem paths — use `t.TempDir()`
- ❌ Flaky tests — fix the root cause, don't retry in a loop
- ❌ Skip `t.Parallel()` on independent unit tests
- ❌ Hard-code ports in tests — use `:0` for ephemeral assignment
- ❌ Leave test containers running — always `defer cleanup()`
- ❌ Skip i18n for any user-visible string

## CRITICAL - ALWAYS DO
- ✅ All tests run in Docker (`make test`)
- ✅ `t.TempDir()` for any test that needs a filesystem
- ✅ Table-driven tests for all data-driven logic
- ✅ `t.Parallel()` for independent tests
- ✅ Mock external API calls — never hit real endpoints in tests
- ✅ Test coverage target: 80%+ for business logic
- ✅ All user-facing strings: `i18n.T(ctx, "key")` — never hardcode
- ✅ i18n keys: `snake_case`, namespaced (`admin.settings.saved`)
- ✅ WCAG 2.1 AA — test with `axe` or equivalent in CI

## TEST STRUCTURE
```
src/
├── util/
│   ├── blocklist_test.go
│   ├── directories_test.go
│   └── username_test.go
└── server/
    └── handler/
        └── *_test.go
```

## TABLE-DRIVEN TEST PATTERN
```go
func TestFoo(t *testing.T) {
    t.Parallel()
    cases := []struct {
        name string
        in   string
        want string
    }{
        {"empty", "", ""},
        {"normal", "hello", "hello"},
    }
    for _, tc := range cases {
        tc := tc
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            got := Foo(tc.in)
            require.Equal(t, tc.want, got)
        })
    }
}
```

## I18N RULES (PART 31)
- Supported languages: English, Spanish, French, German, Arabic, Japanese, Chinese
- Locale files: `src/common/i18n/locales/{lang}.json`
- RTL support: Arabic (ar)
- `i18n.T(ctx, "key")` — always use context for locale detection
- Never hardcode user-visible strings

## DOCUMENTATION (PART 30)
- ReadTheDocs-compatible: `docs/` directory with `mkdocs.yml`
- API docs auto-generated from OpenAPI/Swagger annotations
- `go doc` compatible godoc comments on all exported symbols

---
For complete details, see AI.md PART 29, 30, 31
