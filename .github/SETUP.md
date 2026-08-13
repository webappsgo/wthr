# GitHub Repository Setup for CI/CD

## Required Repository Settings

To enable automatic Docker builds and releases, configure these repository settings:

### 1. Enable GitHub Actions

Go to: **Settings** → **Actions** → **General**

- ✅ Allow all actions and reusable workflows
- ✅ Read and write permissions for workflows
- ✅ Allow GitHub Actions to create and approve pull requests

### 2. Configure Package Permissions

Go to: **Settings** → **Actions** → **General** → **Workflow permissions**

- ✅ Select: **Read and write permissions**
- ✅ Check: **Allow GitHub Actions to create and approve pull requests**

### 3. Package Visibility (Optional)

The workflow automatically sets packages to public, but you can also:

Go to: **Packages** → Click on `wthr` → **Package settings**

- Set visibility to **Public**

### 4. Enable Workflow Permissions (REQUIRED)

**This is required to fix the `permission_denied: write_package` error.**

Go to: **Repository Settings** → **Actions** → **General** → **Workflow permissions**

1. Select: ✅ **Read and write permissions**
2. Check: ✅ **Allow GitHub Actions to create and approve pull requests**
3. Click **Save**

**Additional for Organization Repositories:**

If the repository is under an organization (webappsgo), also check:

Go to: **Organization Settings** → **Actions** → **General**

1. Under **Policies**:
   - Allow actions and reusable workflows
2. Under **Workflow permissions**:
   - Select: **Read and write permissions**

This allows workflows to push Docker images to ghcr.io without needing a personal access token.

---

## Workflow Overview

### `docker-build.yml` - Docker Image Builds
- **Trigger**: Every push to main/develop, or any tag
- **Output**: Multi-platform Docker images (amd64, arm64)
- **Tags**:
  - Regular push: `:latest`, `:YYMM`
  - Tag push: `:latest`, `:version`, `:YYMM`

### `release.yml` - GitHub Releases
- **Trigger**: Tag pushes only
- **Output**: 8 platform binaries
- **Behavior**: Deletes existing release/tag before creating new one

---

## Troubleshooting

### "permission_denied: write_package"

**Solution**: Enable workflow write permissions:
1. Go to **Settings** → **Actions** → **General**
2. Scroll to **Workflow permissions**
3. Select **Read and write permissions**
4. Click **Save**

### "Package already exists"

**Solution**: The workflow automatically deletes existing releases/tags.
If manual intervention is needed:
```bash
# Delete release
gh release delete v1.0.0 -y

# Delete tag locally and remotely
git tag -d v1.0.0
git push origin :refs/tags/v1.0.0
```

### Testing Workflows Locally

Use [act](https://github.com/nektos/act) to test workflows locally:
```bash
# Install act
brew install act  # macOS
# or
curl https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash

# Test docker build workflow
act push -W .github/workflows/docker-build.yml

# Test release workflow
act push -W .github/workflows/release.yml
```

---

## Manual Builds

If you need to build/push manually:

```bash
# Build and push Docker (make target)
make docker

# Create GitHub release (make target)
make release

# Build all binaries
make build
```
