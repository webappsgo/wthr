# Contributing to Weather

## Local Setup

**All builds run in Docker — no Go installation required on your machine.**

```bash
# Clone
git clone https://github.com/apimgr/weather.git
cd weather

# Build (all 8 platforms)
make build

# Build local binary only (fast)
make local

# Run tests
make test

# Lint
make lint

# Run development server
make dev
```

## Branch & PR Workflow

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Make your changes with tests
4. Run `make test` and `make lint` — both must pass
5. Update documentation in `docs/` if behavior changes
6. Open a pull request against `main`

## Code Requirements

- All tests must pass: `make test`
- Lint must be clean: `make lint`
- New behavior requires a test that fails before and passes after
- Documentation in `docs/` must be updated for any user-facing, admin-facing, or API-facing changes
- Swagger annotations must be added for any new REST endpoints
- GraphQL schema must be updated for any schema changes
- Config options must be documented in `docs/configuration.md`

## Security Issues

**Do not open a public issue for security vulnerabilities.**

See `.github/SECURITY.md` for the private reporting path. Vulnerabilities are NOT filed as public bug reports.

## License

By contributing, you agree your contributions will be licensed under the MIT License.
