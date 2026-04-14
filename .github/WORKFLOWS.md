# GitHub Actions Workflows

This document describes the automated GitHub Actions workflows configured for abc-node-probe.

## Workflows

### 1. CI (Continuous Integration)
**File:** `.github/workflows/ci.yml`

**Triggers:**
- Push to `master`, `main`, or `develop` branches
- Pull requests targeting `master`, `main`, or `develop` branches

**Jobs:**

#### Test
- **Runs on:** Ubuntu Latest
- **Go versions:** 1.22, 1.23, 1.24, 1.25, 1.26
- **Steps:**
  - Checkout code
  - Setup Go with version from go.mod
  - Run unit tests (`make test-unit`)
  - Run all tests (`make test`)

#### Lint
- **Runs on:** Ubuntu Latest
- **Tool:** golangci-lint (latest version)
- **Steps:**
  - Checkout code
  - Setup Go with version from go.mod
  - Install and run golangci-lint (`make lint`)

#### Build
- **Runs on:** Ubuntu Latest
- **Platforms:** 
  - Linux (amd64, arm64)
  - macOS/Darwin (amd64, arm64)
  - Windows (amd64, arm64)
- **Steps:**
  - Checkout code
  - Setup Go with version from go.mod
  - Cross-compile binaries for all platforms
  - Upload build artifacts

---

### 2. Build and Release
**File:** `.github/workflows/build-release.yml`

**Triggers:**
- Push of version tags matching `v*.*.*` (e.g., `v1.0.0`, `v1.2.3`)

**Jobs:**

#### Build
- **Runs on:** Ubuntu Latest
- **Platforms:**
  - Linux (amd64, arm64)
  - macOS/Darwin (amd64, arm64)
  - Windows (amd64, arm64)
- **Steps:**
  - Checkout code with full history
  - Setup Go with version from go.mod
  - Cross-compile binaries with version/commit information injected via ldflags
  - Upload build artifacts

#### Release
- **Runs on:** Ubuntu Latest
- **Permissions:** `write` to contents (to create releases)
- **Steps:**
  - Checkout code with full history
  - Setup Go
  - Download all build artifacts from build job
  - Generate SHA256 checksums for all binaries
  - Create GitHub Release with:
    - All binary artifacts
    - SHA256 checksums file
    - Auto-generated release notes with commit info

**Release Assets:**
- `abc-node-probe-linux-amd64`
- `abc-node-probe-linux-arm64`
- `abc-node-probe-darwin-amd64`
- `abc-node-probe-darwin-arm64`
- `abc-node-probe-windows-amd64.exe`
- `abc-node-probe-windows-arm64.exe`
- `sha256sums.txt` (checksums for all binaries)

---

### 3. Vulnerability Check
**File:** `.github/workflows/vulnerability-check.yml`

**Triggers:**
- Push to `master`, `main`, or `develop` branches
- Pull requests targeting `master`, `main`, or `develop` branches
- Daily schedule at 6 AM UTC (helps catch new vulnerabilities in dependencies)

**Jobs:**

#### Go Vulnerability Check (govulncheck)
- **Runs on:** Ubuntu Latest
- **Tool:** govulncheck (from golang.org/x/vuln)
- **Steps:**
  - Checkout code
  - Setup Go
  - Install govulncheck
  - Scan for known vulnerabilities in Go dependencies

#### Trivy Vulnerability Scanner
- **Runs on:** Ubuntu Latest
- **Tool:** Trivy (aquasecurity)
- **Output:** SARIF format, uploaded to GitHub Security
- **Steps:**
  - Checkout code
  - Attempt to build and scan Docker image (if Dockerfile exists)
  - Run Trivy filesystem scan
  - Upload results to GitHub Security tab for visibility

---

## Usage

### Creating a Release

To create a new release of abc-node-probe:

```bash
# 1. Tag the commit with a semantic version
git tag v1.2.3

# 2. Push the tag to trigger the build-release workflow
git push origin v1.2.3
```

The GitHub Actions workflow will:
1. Build binaries for all platforms/architectures
2. Generate checksums
3. Create a GitHub Release with all artifacts
4. Make the binaries available for download

### Checking Workflow Status

- **CI workflows:** Check "Actions" tab in GitHub repository to see test/lint results
- **Release builds:** Automatically create GitHub Releases when version tags are pushed
- **Vulnerability scans:** Check "Security" > "Code scanning" to see results

### Disabling Workflows

To temporarily disable a workflow:
1. Go to "Actions" tab in GitHub
2. Select the workflow
3. Click the "..." menu
4. Select "Disable workflow"

To re-enable:
1. Select the workflow
2. Click "Enable workflow"

---

## Matrix Testing

The CI workflow tests against multiple Go versions to ensure compatibility. If a test fails on a specific Go version, check the logs in the Actions tab to see what changed.

## Performance Notes

- CI tests run in parallel for different Go versions
- Build jobs run in parallel for different platforms
- Releasing typically takes 5-10 minutes for all platforms
- Vulnerability checks add ~2-3 minutes per run

---

## Future Enhancements

Potential future improvements to these workflows:

1. **Code Coverage:** Add coverage reporting and track coverage trends
2. **Docker Image:** Build and publish Docker images on release
3. **Binary Signing:** GPG sign released binaries for verification
4. **Integration Tests:** Run integration tests in CI (currently disabled/skipped)
5. **OS-Specific Testing:** Test on actual macOS and Windows runners
6. **Performance Benchmarks:** Track performance benchmarks across versions
7. **SBOM Generation:** Create Software Bill of Materials (SBOM) for releases
