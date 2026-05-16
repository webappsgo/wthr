# CI/CD Rules (PART 28)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Use Makefile in CI/CD — explicit commands only in workflow YAML
- ❌ Pin GitHub Actions by tag (`@v4`) — must use full commit SHA
- ❌ Use `GITHUB_TOKEN` for pushing to other repos — only for the current repo
- ❌ Store secrets in workflow YAML — use GitHub Actions Secrets
- ❌ Build only for one platform — always linux/amd64 + linux/arm64
- ❌ Skip `CGO_ENABLED=0` in CI build commands
- ❌ Push to `latest` on non-tag pushes — `latest` is for tagged releases only
- ❌ Skip QEMU setup for multi-arch Docker builds

## CRITICAL - ALWAYS DO
- ✅ Pin ALL third-party actions to full commit SHA with version comment
  - Format: `uses: owner/action@{sha} # v{version}`
- ✅ Build all 8 binary targets in release/beta/daily workflows
- ✅ Multi-arch Docker: `linux/amd64,linux/arm64` via QEMU + Buildx
- ✅ `CGO_ENABLED=0` explicit in every build step
- ✅ `-ldflags="-s -w"` + all 4 version flags in every build
- ✅ Push Docker `latest` tag ONLY on git tag pushes (IS_TAG=true)
- ✅ AIO image uses `-aio` suffix: `latest-aio`, `devel-aio`, etc.

## WORKFLOW FILES
| File | Trigger | Purpose |
|------|---------|---------|
| `docker.yml` | push (any branch/tag), workflow_dispatch | Build + push Docker images |
| `release.yml` | push tag `v*` or `N.N.N` | Build binaries + create GitHub Release |
| `beta.yml` | push branch `beta` | Build binaries + create pre-release |
| `daily.yml` | schedule 3am UTC + push main/master | Rolling daily release |

## DOCKER TAG STRATEGY
| Git event | Standard tags | AIO tags |
|-----------|--------------|----------|
| Tag push | `{version}`, `latest`, `{YYMM}`, `{commit}` | Same with `-aio` suffix |
| Non-tag push | `devel`, `{commit}` | Same with `-aio` suffix |
| Beta branch | `beta`, `devel`, `{commit}` | Same with `-aio` suffix |

## SHA-PINNED ACTIONS (current pins)
| Action | SHA | Version |
|--------|-----|---------|
| `actions/checkout` | `de0fac2e4500dabe0009e67214ff5f5447ce83dd` | v6 |
| `actions/setup-go` | `40f1582b2485089dde7abd97c1529aa768e1baff` | v5 |
| `actions/upload-artifact` | `330a01c490aca151604b8cf639adc76d48f6c5d4` | v5 |
| `actions/download-artifact` | `634f93cb2916e3fdff6788551b99b062d0335ce0` | v5 |
| `softprops/action-gh-release` | `3bb12739c298aeb8a4eeaf626c5b8d85266b0e65` | v2 |
| `docker/setup-qemu-action` | `c7c53464625b32c7a7e944ae62b3e17d2b600130` | v3 |
| `docker/setup-buildx-action` | `8d2750c68a42422c14e847fe6c8ac0403b4cbd6f` | v3 |
| `docker/login-action` | `c94ce9fb468520275223c153574b00df6fe4bcc9` | v3 |
| `docker/build-push-action` | `10e90e3645eae34f1e60eeb005ba3a3d33f178e8` | v6 |

---
For complete details, see AI.md PART 28
