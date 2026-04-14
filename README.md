# abc-node-probe

A static, read-only binary that assesses whether a node is ready to join the ABC-cluster Tailscale network. Run it before onboarding a node — manually or as part of an automated pipeline.

It collects structured health-check results and either prints them to stdout, writes them to a JSON file, or POSTs them to the ABC-cluster control plane API.

**It never modifies system state.**

📖 **Quick Start:** See [USAGE.md](USAGE.md) for installation, common workflows, and examples.

---

## Getting Started

### Installation

The easiest way is via the abc-cluster-cli, which automatically downloads the latest release:

```bash
# Install via CLI — automatically fetches latest release from GitHub
abc compute probe <node-id>
```

Or download directly from [GitHub Releases](https://github.com/abc-cluster/abc-node-probe/releases):

```bash
# Get the v0.1.0 release for your platform
wget https://github.com/abc-cluster/abc-node-probe/releases/download/v0.1.0/abc-node-probe-linux-amd64
chmod +x abc-node-probe-linux-amd64
./abc-node-probe-linux-amd64 --version
```

### First Run (2 minutes)

```bash
# Declare jurisdiction and run the probe
./abc-node-probe --jurisdiction=ZA

# Output shows a color-coded table plus JSON report
# Look for: "NODE ELIGIBLE TO JOIN: YES" at the bottom
```

---

## Check categories

| Category     | What it checks |
|--------------|----------------|
| `hardware`   | CPU architecture, core count, RAM, NUMA topology, GPU devices |
| `storage`    | Scratch free space, inode availability, write throughput, MinIO endpoint, inotify limits, open-file ulimits |
| `smart`      | Drive health via S.M.A.R.T. ioctl (Linux only; ATA, NVMe, SCSI) |
| `network`    | Tailscale daemon, NTP sync, DNS resolution, network interfaces, **network throughput (speedtest)** |
| `os`         | Kernel version, cgroups version, kernel namespaces, systemd, SELinux, overlayfs |
| `compliance` | Jurisdiction declaration, encryption at rest (LUKS), cross-border mounts |
| `security`   | SSH root login, firewall, world-writable system dirs, TLS cert expiry |

### Network Speedtest

The `network.speedtest.throughput` check measures actual network performance by running a speed test against the nearest `speedtest.net` server. This provides:

- **Download speed** (Mbps)
- **Upload speed** (Mbps)
- **Latency** (milliseconds)
- **Jitter** (milliseconds)
- **Server location and distance**
- **Client ISP and public IP** (metadata only)

**Duration:** ~30-60 seconds depending on network speed  
**Severity:** INFO (or WARN if download < 10 Mbps)  
**Requirements:** Internet connectivity to speedtest.net servers

This check is informational and does not prevent cluster membership, but provides valuable insights into node network connectivity and performance before onboarding.

---

## Requirements

- Go 1.22 or newer
- `CGO_ENABLED=0` — the build is always static; CGO is never used
- Target OS: Linux, macOS, or Windows
- For S.M.A.R.T. checks (Linux only): run as root, or be a member of the `disk` group

---

## Build

### From source

```sh
# Build for the host platform
make build

# Cross-compile for a specific platform
make build-linux-amd64
make build-linux-arm64
make build-darwin-amd64
make build-darwin-arm64
make build-windows-amd64
make build-windows-arm64

# Build all platforms at once
make build-all
```

The binary will appear as `./abc-node-probe` (host) or `dist/abc-node-probe-<os>-<arch>[.exe]` (cross-compiled).

**Version Injection:** The Makefile automatically injects version from git tags:

```bash
# Building from a tagged commit
git describe --tags
# → v1.0.0

make build
# Produces: abc-node-probe v1.0.0

# Without a tag, uses "dev" as version
git checkout main && make build
# Produces: abc-node-probe dev
```

### Release

```sh
# Build binaries for all platforms
make build-all

# Generate checksums
cd dist && sha256sum abc-node-probe-* > sha256sums.txt

# Push tag to GitHub (triggers CI release workflow)
git tag -a v1.0.0 -m "Release abc-node-probe v1.0.0"
git push origin v1.0.0
```

The GitHub Actions [build-release.yml](.github/workflows/build-release.yml) workflow automatically:
1. Builds binaries for all platforms
2. Generates checksums
3. Creates a GitHub Release with artifacts

The cli will automatically fetch and cache the latest release when running `abc compute probe`.

### Verify static binary

To confirm the Linux binary is fully static:

```sh
file dist/abc-node-probe-linux-amd64
# → abc-node-probe-linux-amd64: ELF 64-bit LSB executable, ... statically linked

ldd dist/abc-node-probe-linux-amd64
# → not a dynamic executable
```

---

## Test

```sh
# Unit tests only — no network, no /proc required, works on any OS
make test-unit

# All tests (includes tests that read local system state)
make test

# Integration tests (Linux only, tagged //go:build integration)
CGO_ENABLED=0 go test -tags integration ./...
```

---

## Run

See [USAGE.md](USAGE.md) for detailed workflows, examples, and integration guides.

### Quick reference

```sh
# Basic run — stdout mode, no compliance check
./abc-node-probe

# With jurisdiction declared (required for compliance checks)
./abc-node-probe --jurisdiction=ZA

# Nomad-compatible mode (always exits 0, check JSON for readiness)
./abc-node-probe --nomad-mode --jurisdiction=ZA --json

# Write JSON report to file
./abc-node-probe --jurisdiction=ZA --mode=file --output-file=/tmp/report.json

# Send to control plane API
./abc-node-probe --jurisdiction=ZA --mode=send --api-endpoint=https://api.abc-cluster.example.com

# Send results to API with nomad-mode (exits 0 but posts results)
./abc-node-probe --nomad-mode --jurisdiction=ZA --mode=send --api-endpoint=https://api.abc-cluster.example.com

# Skip slow or irrelevant categories
./abc-node-probe --jurisdiction=ZA --skip-categories=smart,compliance

# JSON-only output (no colour table)
./abc-node-probe --jurisdiction=ZA --json

# Stop on first failure
./abc-node-probe --jurisdiction=ZA --fail-fast

# Print version
./abc-node-probe --version
```

For more examples, including CI/CD integration, batch testing via Nomad, and API workflows, see **[USAGE.md](USAGE.md)**.

### Environment variables

| Variable | Equivalent flag | Notes |
|---|---|---|
| `ABC_PROBE_TOKEN` | `--api-token` | Bearer token for control plane API |
| `ABC_PROBE_API` | `--api-endpoint` | Control plane API base URL |
| `ABC_PROBE_JURISDICTION` | `--jurisdiction` | ISO 3166-1 alpha-2 country code |
| `ABC_MINIO_ENDPOINT` | — | `host:port` of MinIO endpoint to test; storage check is skipped if unset |

---

## Output

### Stdout (default)

```
abc-node-probe v0.1.0 — node: worker-01 — role: compute — jurisdiction: ZA
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
CATEGORY       CHECK                          SEVERITY   VALUE          MESSAGE
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
hardware       cpu.architecture               PASS       x86_64
hardware       memory.total_ram               INFO       256.0 GB
network        tailscale.daemon_running       PASS
network        ntp.sync_status                PASS
compliance     jurisdiction.declared          PASS       ZA
...
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
SUMMARY: 28 checks — 22 PASS, 3 WARN, 0 FAIL, 3 SKIP
NODE ELIGIBLE TO JOIN: YES
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

{ ... full JSON report ... }
```

Colour is suppressed automatically when stdout is not a TTY (e.g. when piping to `jq`).

### JSON report schema

Every run produces one `ProbeReport`:

```json
{
  "schema_version": "1.0",
  "probe_version": "0.1.0",
  "node_hostname": "worker-01",
  "node_role": "compute",
  "jurisdiction": "ZA",
  "timestamp": "2026-04-14T14:22:00Z",
  "total_duration_ms": 4321,
  "summary": {
    "total_checks": 28,
    "pass_count": 22,
    "warn_count": 3,
    "fail_count": 0,
    "skip_count": 3,
    "info_count": 0,
    "eligible_to_join": true
  },
  "results": [
    {
      "id": "hardware.cpu.architecture",
      "category": "hardware",
      "name": "CPU Architecture",
      "severity": "PASS",
      "message": "",
      "value": "x86_64",
      "duration_ms": 2
    }
  ]
}
```

---

## Exit codes

| Code | Meaning |
|------|---------|
| `0`  | All checks PASS or SKIP, OR `--nomad-mode` is enabled |
| `1`  | At least one WARN, zero FAIL (not used in `--nomad-mode`) |
| `2`  | At least one FAIL (not used in `--nomad-mode`) |
| `3`  | Tool execution error (bad flags, API unreachable, file write failure) |

### Using with Nomad

When running abc-node-probe as a Nomad job via `raw_exec` driver, use the `--nomad-mode` flag to ensure the job exits cleanly with code 0 regardless of probe results. This prevents Nomad from restarting the task on node health issues.

```hcl
task "probe" {
  driver = "raw_exec"
  config {
    command = "/opt/nomad/abc-node-probe"
    args = [
      "--nomad-mode",
      "--json",
      "--jurisdiction=ZA"
    ]
  }
}
```

In `--nomad-mode`:
- **Exit code is always 0** (task completed successfully)
- **Node readiness verdict** is conveyed via JSON output in the `summary.eligible_to_join` field
- **Allocations can be queried** for the probe output (`nomad alloc logs <alloc-id> probe`) to determine actual node readiness
- **API mode (`--mode=send`)** can be combined with `--nomad-mode`: results are posted to control plane and job still exits 0

**Example: Send results to API with clean Nomad exit**
```bash
./abc-node-probe --nomad-mode --mode=send --jurisdiction=ZA --api-endpoint=https://api.abc-cluster --api-token=$TOKEN
```

---

## Release

### Publishing a new release

1. **Tag the commit** with semantic versioning:
   ```bash
   git tag -a v1.1.0 -m "Release v1.1.0: Add new compliance checks"
   git push origin v1.1.0
   ```

2. **GitHub Actions** automatically:
   - Builds binaries for all platforms
   - Generates SHA256 checksums
   - Creates a GitHub Release with artifacts

3. **The abc-cluster-cli** automatically:
   - Detects the new release via GitHub API
   - Downloads and caches binaries
   - Updates `probe_version` in outputs

### Release artifacts

Each release includes:
- `abc-node-probe-linux-amd64` — Linux x86_64
- `abc-node-probe-linux-arm64` — Linux ARM64
- `abc-node-probe-darwin-amd64` — macOS Intel
- `abc-node-probe-darwin-arm64` — macOS Apple Silicon
- `abc-node-probe-windows-amd64.exe` — Windows x86_64
- `abc-node-probe-windows-arm64.exe` — Windows ARM64
- `sha256sums.txt` — Checksums for verification

### Versioning scheme

Uses [Semantic Versioning](https://semver.org/):
- `v0.1.0` — Initial release
- `v0.2.0` — New features (backward compatible)
- `v0.1.1` — Bug fixes (backward compatible)
- `v1.0.0` — Stable release

---

## Lint

```sh
make lint   # requires golangci-lint in PATH
```

---

## Documentation

- **[USAGE.md](USAGE.md)** — Detailed usage guide with workflows and examples
  - Installation from releases
  - Common use cases (pre-flight checks, batch testing, CI/CD)
  - Nomad integration
  - API integration with control plane
  - Troubleshooting

- **[docs/](docs/)** — Architecture and design documents
  - Check category details
  - JSON schema reference
  - Hardware compatibility notes

---

## Support

Issues and feature requests: [GitHub Issues](https://github.com/abc-cluster/abc-node-probe/issues)

When reporting issues, include:
- Output of `./abc-node-probe --version`
- Full JSON report: `./abc-node-probe --jurisdiction=ZA --json`
- OS and kernel information: `uname -a`

