# CLAUDE.md — nomad-node-probe

## Project Overview

Implement `nomad-node-probe`, a single static Go binary that assesses a Linux node's readiness to join the ABC-cluster Nomad/Tailscale hybrid compute network. The tool runs pre-join (manually by a sysadmin or as part of an automated onboarding pipeline), collects structured health check results, and either prints them to stdout, writes them to a file, or POSTs them to the ABC-cluster control plane API.

This tool is a **read-only assessment instrument**. It must never modify system state.

---

## Repository Layout

```
nomad-node-probe/
├── CLAUDE.md                   # this file
├── go.mod
├── go.sum
├── main.go                     # CLI entrypoint, flag parsing, mode dispatch
├── cmd/
│   └── root.go                 # cobra root command wiring
├── probe/
│   ├── runner.go               # orchestrates all check categories, collects ProbeReport
│   ├── result.go               # CheckResult, ProbeReport types and JSON schema
│   ├── hardware.go             # CPU, RAM, NUMA, GPU checks
│   ├── storage.go              # disk, mounts, MinIO, inotify, ulimits
│   ├── smart.go                # S.M.A.R.T. drive health via anatol/smart.go ioctl
│   ├── network.go              # Tailscale, Nomad, NTP, DNS, ports
│   ├── os.go                   # kernel, cgroups, namespaces, systemd, SELinux
│   ├── nomad.go                # Nomad binary, version, config, drivers
│   ├── nextflow.go             # Java, rclone, Singularity (optional checks)
│   ├── compliance.go           # jurisdiction, OPA, cross-border mounts, encryption
│   └── security.go             # SSH config, firewall, world-writable dirs, TLS
├── output/
│   ├── printer.go              # coloured terminal renderer (PASS/WARN/FAIL/SKIP)
│   └── sender.go               # HTTP POST to control plane API
├── internal/
│   ├── proc.go                 # helpers for reading /proc and /sys paths
│   └── exec.go                 # safe subprocess execution with timeout
└── testdata/
    └── fixtures/               # mock /proc file content for unit tests
```

---

## Build Requirements

- Go 1.22 or newer
- `CGO_ENABLED=0` — mandatory for static binary; all builds must use this flag
- Target: `GOOS=linux`, primary architectures `GOARCH=amd64` and `GOARCH=arm64`
- The Makefile `build` target must set `CGO_ENABLED=0` and pass `-ldflags="-s -w"` to strip debug symbols
- Binary must run on Linux kernel 3.x or newer, Ubuntu 14.04 LTS and later

### Makefile Targets (implement these)

```makefile
build          # CGO_ENABLED=0 go build for host arch
build-amd64    # cross-compile linux/amd64
build-arm64    # cross-compile linux/arm64
test           # go test ./...
test-unit      # go test ./... -short (no network, no /proc required)
lint           # golangci-lint run
release        # build both arches, sha256sum, produce dist/
```

---

## Dependencies

Add exactly these modules and no others without justification in a comment:

```
github.com/shirou/gopsutil/v3      # CPU, memory, disk, network cross-platform
github.com/jaypipes/ghw            # NUMA, PCI/GPU enumeration, block device enumeration
github.com/vishvananda/netlink     # MTU, interface inspection via netlink socket
golang.org/x/sys                   # cgroups, kernel namespaces, low-level syscalls
github.com/coreos/go-systemd/v22   # systemd unit status
github.com/hashicorp/hcl/v2        # Nomad HCL config file parsing/validation
github.com/spf13/cobra             # CLI flag parsing
github.com/fatih/color             # terminal colour output (PASS=green, WARN=yellow, FAIL=red)
github.com/anatol/smart.go         # pure Go S.M.A.R.T. ioctl passthrough — ATA, NVMe, SCSI
```

Do not add:
- Any osquery dependency
- Any GUI or TUI framework
- Any database driver
- Any cloud SDK

---

## CLI Interface

```
nomad-node-probe [flags]

Flags:
  --jurisdiction string     ISO 3166-1 alpha-2 country code where this node is physically located.
                            REQUIRED for compliance checks. Example: --jurisdiction=ZA
  --node-role string        One of: compute, storage, scheduler, gateway. Default: compute
  --nomad-server string     Nomad server address to test reachability. Format: host:port. Default: 127.0.0.1:4647
  --nomad-config string     Path to Nomad client config file to validate. Default: /etc/nomad.d/client.hcl
  --api-endpoint string     ABC-cluster control plane API base URL. Required for --mode=send
  --api-token string        Bearer token for control plane API authentication. Read from ABC_PROBE_TOKEN env var if not set.
  --mode string             Output mode: stdout | file | send. Default: stdout
  --output-file string      Path to write JSON report. Required for --mode=file
  --skip-categories string  Comma-separated list of check categories to skip.
                            Valid values: hardware,storage,smart,network,os,nomad,nextflow,compliance,security
  --fail-fast               Stop after first FAIL result instead of running all checks
  --json                    Print raw JSON to stdout even in stdout mode (suppresses coloured output)
  --timeout duration        Overall probe timeout. Default: 120s
  --version                 Print version and exit
```

### Environment Variables

| Variable | Purpose |
|---|---|
| `ABC_PROBE_TOKEN` | Bearer token for API authentication (preferred over flag for security) |
| `ABC_PROBE_API` | Control plane API base URL (overridden by --api-endpoint) |
| `ABC_PROBE_JURISDICTION` | Jurisdiction code (overridden by --jurisdiction) |

---

## JSON Report Schema

The canonical output type. Every execution produces exactly one `ProbeReport` serialised as JSON.

```go
// result.go

type Severity string

const (
    SeverityPass Severity = "PASS"
    SeverityWarn Severity = "WARN"
    SeverityFail Severity = "FAIL"
    SeveritySkip Severity = "SKIP"
    SeverityInfo Severity = "INFO"
)

type CheckResult struct {
    ID          string            `json:"id"`            // e.g. "storage.scratch.free_space"
    Category    string            `json:"category"`      // e.g. "storage"
    Name        string            `json:"name"`          // human-readable check name
    Severity    Severity          `json:"severity"`
    Message     string            `json:"message"`       // explains the result
    Value       interface{}       `json:"value,omitempty"` // measured value where applicable
    Unit        string            `json:"unit,omitempty"`  // e.g. "GB", "MB/s", "cores"
    Metadata    map[string]string `json:"metadata,omitempty"` // arbitrary key-value context
    DurationMs  int64             `json:"duration_ms"`   // how long this check took
}

type ProbeReport struct {
    SchemaVersion string        `json:"schema_version"`  // "1.0"
    ProbeVersion  string        `json:"probe_version"`   // injected at build time via ldflags
    NodeHostname  string        `json:"node_hostname"`
    NodeRole      string        `json:"node_role"`
    Jurisdiction  string        `json:"jurisdiction"`    // as declared, not inferred
    Timestamp     time.Time     `json:"timestamp"`       // UTC
    DurationMs    int64         `json:"total_duration_ms"`
    Summary       ProbeSummary  `json:"summary"`
    Results       []CheckResult `json:"results"`
}

type ProbeSummary struct {
    TotalChecks  int `json:"total_checks"`
    PassCount    int `json:"pass_count"`
    WarnCount    int `json:"warn_count"`
    FailCount    int `json:"fail_count"`
    SkipCount    int `json:"skip_count"`
    InfoCount    int `json:"info_count"`
    Eligible     bool `json:"eligible_to_join"` // true only if FailCount == 0
}
```

### Check ID Naming Convention

IDs follow the pattern `{category}.{subcategory}.{check_name}`, all lowercase with underscores. Examples:

```
hardware.cpu.architecture
hardware.memory.total_ram
storage.scratch.free_space
storage.minio.endpoint_reachable
network.tailscale.daemon_running
network.ntp.sync_status
os.kernel.namespaces_available
os.cgroups.version
nomad.binary.version
nomad.config.parseable
storage.smart.overall_health
storage.smart.reallocated_sectors
storage.smart.pending_sectors
storage.smart.uncorrectable_sectors
storage.smart.power_on_hours
storage.smart.temperature
storage.smart.ssd_life_left
storage.smart.nvme_critical_warning
storage.smart.nvme_available_spare
storage.smart.nvme_percentage_used
storage.smart.nvme_media_errors
compliance.jurisdiction.declared
compliance.encryption.data_at_rest
security.ssh.root_login_disabled
```

---

## Output Modes

### Mode: stdout (default)

Print a coloured human-readable table to stdout, then print the full JSON report to stdout beneath it. Use `github.com/fatih/color` for PASS=green, WARN=yellow, FAIL=red, SKIP=grey, INFO=cyan.

Format:
```
nomad-node-probe v0.1.0 — node: lengau-login01 — role: compute — jurisdiction: ZA
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
CATEGORY       CHECK                          SEVERITY   VALUE          MESSAGE
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
hardware       cpu.architecture               PASS       x86_64
hardware       memory.total_ram               INFO       256 GB
storage        scratch.free_space             PASS       1.2 TB
storage        minio.endpoint_reachable       FAIL                      No route to host: 10.0.1.5:9000
network        tailscale.daemon_running       PASS
network        ntp.sync_status                WARN                      Clock offset 1.8s — within tolerance but approaching limit
...
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
SUMMARY: 34 checks — 28 PASS, 4 WARN, 1 FAIL, 1 SKIP
NODE ELIGIBLE TO JOIN: NO (resolve FAIL checks first)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

{ ... full JSON report ... }
```

If `--json` flag is set, suppress the coloured table and print only raw JSON.

### Mode: file

Write the JSON report to `--output-file` path. Print a one-line summary to stdout:
```
Report written to /tmp/probe-lengau-login01-20260317T142200Z.json
Summary: 34 checks — 28 PASS, 4 WARN, 1 FAIL — NOT ELIGIBLE
```

### Mode: send

POST the JSON report to `{api-endpoint}/v1/nodes/probe` with:
- `Content-Type: application/json`
- `Authorization: Bearer {token}`
- Timeout: 30 seconds
- Retry: 3 attempts with exponential backoff (1s, 2s, 4s)

Also print the coloured human-readable table to stdout (same as stdout mode).

On success, print the API response body (which will contain a `node_id` and `admission_status` from the control plane). On failure, print the HTTP status and response body, and exit with code 2.

**API request body:** the full `ProbeReport` JSON.

**API response schema (for display purposes — do not validate strictly):**
```json
{
  "node_id": "abc-node-uuid",
  "admission_status": "pending_review | approved | rejected",
  "message": "string"
}
```

---

## Check Implementation Requirements

### General Rules for All Checks

1. Every check function must return a `CheckResult` and must never panic
2. Every check must record its own duration using `time.Since(start)`
3. Checks that require root and are running as non-root must return `SKIP` with message `"requires root — skipping"`
4. Checks that are not applicable to the declared `--node-role` must return `SKIP`
5. Use `internal/proc.go` helpers for all `/proc` and `/sys` reads — never use `os/exec` to shell out to system utilities except where explicitly noted below
6. All subprocess calls (via `internal/exec.go`) must have a timeout of 10 seconds maximum
7. No check may write to disk, modify any system file, or change any system state
8. Network checks must use `net.DialTimeout` — never use `ping` via subprocess

### Permitted subprocess calls (via internal/exec.go only)

Some information is only accessible via subprocess. These are the only permitted cases:

| Check | Command | Reason |
|---|---|---|
| NTP sync status | `timedatectl show` | No /proc equivalent for NTP offset |
| Tailscale status | `tailscale status --json` | Tailscale local API JSON |
| Disk encryption | `dmsetup info` | LUKS detection |
| Java version | `java -version` | Runtime version string |
| rclone version | `rclone version` | Plugin compatibility check |
| Nomad version | `nomad version` | Binary version string |
| Singularity version | `singularity version` | Optional presence check |

For all other checks, read directly from `/proc`, `/sys`, or use gopsutil/ghw.

### Scratch IOPS Check Implementation

Do not shell out to `fio` or `dd`. Implement as follows:
1. Create a temp file in the scratch directory
2. Write 512MB in 4MB sequential chunks using `os.File.Write` + `syscall.Fsync`
3. Record wall-clock duration
4. Delete the temp file
5. Return MB/s as `Value`, WARN if < 100 MB/s

### Compliance: Jurisdiction Check

The `compliance.jurisdiction.declared` check must:
- FAIL if `--jurisdiction` was not provided
- Never attempt to infer jurisdiction from IP geolocation or any network lookup
- Record the declared value in `CheckResult.Metadata["declared_by"] = "operator_flag"`
- The message must include: `"Jurisdiction declared by operator. Value not independently verified."`

---

## S.M.A.R.T. Check Implementation (`probe/smart.go`)

### Protocol

Use `github.com/anatol/smart.go` exclusively. Do not shell out to `smartctl` under any circumstances — it may not be installed and its presence cannot be assumed.

The library issues ioctl calls directly to block device file descriptors:
- SATA/ATA: `SG_IO` ioctl with `ATA PASS-THROUGH (16)` on `/dev/sdX`
- NVMe: `NVME_IOCTL_ADMIN_CMD` ioctl on `/dev/nvmeX` (the controller device, **not** the namespace `/dev/nvmeXnY`)
- SCSI/SAS: `LOG SENSE` command via `SG_IO`

### Drive Enumeration

Use `ghw.Block()` to enumerate physical block devices. Filter out before attempting any SMART read:
- Loop devices (`/dev/loopX`)
- Device mapper devices (`/dev/dm-X`)
- RAM block devices (`/dev/ramX`)
- Partition devices (any device where the name ends in a digit following a non-digit, e.g. `/dev/sda1`)

Attempt a SMART read on each remaining device. Handle errors per-device: a failed read on one device must never prevent checks on other devices.

### Per-Device Result Pattern

Each SMART check produces **one `CheckResult` per physical device**, not one per node. Every SMART `CheckResult` must populate `Metadata` with:

```
device  = /dev/sda          (or /dev/nvme0, /dev/sdb, etc.)
model   = <drive model string from IDENTIFY DEVICE>
serial  = <serial number from IDENTIFY DEVICE>
type    = ata | nvme | scsi
```

The check ID includes the device name to ensure uniqueness:
```
storage.smart.sda.overall_health
storage.smart.sda.reallocated_sectors
storage.smart.nvme0.nvme_critical_warning
```

### ATA Check Thresholds

```
Attribute 5   — Reallocated Sector Count
                raw == 0         → PASS
                0 < raw <= 10    → WARN
                raw > 10         → FAIL

Attribute 9   — Power-On Hours
                raw <= 40000     → INFO (record value only)
                raw > 40000      → WARN ("drive has > 40,000 powered-on hours")

Attribute 177 — Wear Leveling Count (SSD only)
                current >= 10    → PASS
                current < 10     → WARN

Attribute 187 — Reported Uncorrectable Errors
                raw == 0         → PASS
                raw > 0          → FAIL

Attribute 190 — Airflow Temperature (°C) — present on WD and some others
Attribute 194 — Temperature Celsius — present on most drives
                Check whichever is present; if both present, use 194
                raw <= 55        → PASS (record value as INFO)
                55 < raw <= 60   → WARN
                raw > 60         → FAIL

Attribute 197 — Current Pending Sector Count
                raw == 0         → PASS
                raw > 0          → WARN ("unstable sectors pending reallocation")

Attribute 198 — Uncorrectable Sector Count
                raw == 0         → PASS
                raw > 0          → FAIL ("data loss confirmed at physical level")

Attribute 231 — SSD Life Left (%)
                raw >= 10        → PASS (record percentage as INFO)
                raw < 10         → FAIL
```

Only interpret attributes with IDs present in the above list. Ignore all other vendor-defined attributes — do not attempt to interpret unknown IDs; instead, record them as INFO entries in Metadata if present.

### NVMe Check Thresholds

```
CriticalWarning (bitmask)
                == 0             → PASS
                any bit set      → FAIL (include bitmask value in message)

Temperature     (convert Kelvin → Celsius: value - 273)
                <= 60°C          → PASS (record as INFO)
                > 60°C           → WARN
                > 70°C           → FAIL

AvailableSpare (%)
                >= 10            → PASS
                < 10             → FAIL

PercentageUsed
                <= 90            → PASS (record as INFO)
                > 90             → WARN
                > 99             → FAIL

MediaErrors
                == 0             → PASS
                > 0              → FAIL

NumErrLogEntries — record as INFO only, no WARN/FAIL threshold
```

### Overall Health Verdict

For ATA drives, call the `SMART RETURN STATUS` command. The drive firmware evaluates all its own internal thresholds and returns a single pass/fail verdict. Map directly:
- Firmware says PASSED → `storage.smart.<dev>.overall_health` PASS
- Firmware says FAILED → FAIL with message `"drive firmware predicts imminent failure"`

For NVMe, the `CriticalWarning` field in the health log serves the equivalent purpose.

### Skip Conditions (must check in this order)

1. Running as non-root and not in `disk` group → SKIP all SMART checks with message `"requires root or disk group membership"`
2. `ghw.Block()` returns no physical devices → SKIP with message `"no physical block devices detected"`
3. Device open returns permission error → SKIP that device
4. Device open returns "no such device" or ioctl returns unsupported → SKIP that device with message `"device does not support SMART or passthrough unavailable (possible hardware RAID controller)"`
5. Drive type is unrecognised by the library → SKIP that device with message `"unrecognised drive interface — skipping to avoid misinterpretation"`

A SKIP on one device does not affect other devices.

---

## Runner Orchestration

`probe/runner.go` must:

1. Run check categories in this order: hardware → storage → smart → network → os → nomad → nextflow → compliance → security
2. Run checks **within each category in parallel** using goroutines with a `sync.WaitGroup`
3. Run categories **sequentially** (a failed network category should not block storage checks, but the report order must be deterministic)
4. Apply `--skip-categories` filtering before launching any goroutines for that category
5. Apply `--fail-fast` by checking a shared `atomic.Bool` after each check result is collected; if set, skip remaining checks in the current category and all subsequent categories
6. Collect all results into `ProbeReport.Results` in deterministic order (sort by `CheckResult.ID` within each category)
7. Compute `ProbeSummary` after all checks complete
8. Set `ProbeSummary.Eligible = (FailCount == 0)`

---

## Exit Codes

| Code | Condition |
|---|---|
| 0 | All checks PASS or SKIP (no WARN, no FAIL) |
| 1 | At least one WARN, zero FAIL |
| 2 | At least one FAIL |
| 3 | Tool execution error (bad flags, unreachable API, file write failure) |

---

## Version Injection

Inject version at build time via ldflags. In `main.go`:

```go
var (
    Version   = "dev"
    BuildTime = "unknown"
    GitCommit = "unknown"
)
```

Makefile release target must pass:
```
-ldflags="-s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"
```

---

## Testing Requirements

### Unit Tests (no external dependencies, no network, no /proc required)

- `probe/result_test.go` — JSON serialisation round-trip for `ProbeReport` and `CheckResult`
- `probe/hardware_test.go` — mock gopsutil responses using interfaces; test PASS/WARN/FAIL threshold logic
- `probe/storage_test.go` — test free space threshold logic with mock disk stats
- `probe/network_test.go` — test timeout handling; use `net.Listen` on a local port for reachability checks
- `probe/compliance_test.go` — test jurisdiction FAIL when flag absent, PASS when present, metadata content
- `output/printer_test.go` — test that summary counts and eligibility flag render correctly
- `output/sender_test.go` — use `net/http/httptest` to mock control plane API; test retry logic, token header, and error handling

### Integration Tests (require Linux, may read /proc)

Tag with `//go:build integration` and skip in `test-unit` target.

- `probe/os_integration_test.go` — run on CI Linux runner; assert kernel namespace check returns PASS on modern Ubuntu
- `probe/storage_integration_test.go` — run scratch IOPS benchmark against `/tmp`
- `probe/smart_integration_test.go` — run on CI Linux runner with a known virtual disk; assert SKIP is returned gracefully when no physical SMART-capable device is present; assert per-device Metadata fields are populated when a device is found

### Done When

- [ ] `make build` produces a statically linked binary: `file nomad-node-probe` reports `statically linked`
- [ ] `ldd nomad-node-probe` reports `not a dynamic executable`
- [ ] `./nomad-node-probe --jurisdiction=ZA --mode=stdout` exits without panic on Ubuntu 20.04 and 22.04
- [ ] `./nomad-node-probe --jurisdiction=ZA --mode=file --output-file=/tmp/report.json` produces valid JSON parseable by `jq`
- [ ] JSON output passes schema validation: all required fields present, all `severity` values are valid enum members
- [ ] `--mode=send` POSTs correct JSON with Authorization header (verified by httptest mock)
- [ ] Exit code is `2` when any FAIL check is present
- [ ] Exit code is `1` when only WARN checks are present
- [ ] `make test-unit` passes with `CGO_ENABLED=0`
- [ ] `make build-arm64` produces a valid arm64 ELF binary

---

## Known Pitfalls

**gopsutil on old kernels:** Some gopsutil calls silently return zero values on kernels older than 4.x. Always check for zero-value results and return WARN with message `"could not read — kernel may not support this metric"` rather than treating zero as a valid measurement.

**ghw on non-root:** `ghw.GPU()` and `ghw.Topology()` (NUMA) require root on some kernel configurations. Wrap in a root check and return SKIP if running as non-root.

**Tailscale socket path varies:** The Tailscale daemon socket is at `/var/run/tailscale/tailscaled.sock` on most systems but may be elsewhere. Check `tailscale status --json` via subprocess rather than assuming socket path.

**HCL v2 partial parsing:** `hcl/v2` returns diagnostics rather than errors. A config file can be "parseable" with warnings. Treat any `diag.Severity == hcl.DiagError` as FAIL; `DiagWarning` as WARN.

**timedatectl on non-systemd systems:** On Ubuntu 14.04 with upstart, `timedatectl` does not exist. Check binary presence before calling it; fall back to reading `/proc/driver/rtc` or returning SKIP.

**Colour output on non-TTY:** `github.com/fatih/color` auto-disables colour when stdout is not a TTY. Do not override this behaviour — JSON output piped to `jq` should not contain ANSI escape codes.

**Scratch IOPS temp file cleanup:** Use `defer os.Remove(tmpFile)` immediately after creating the temp file. If the benchmark panics (should not happen, but guard anyway), the defer ensures cleanup.

**/proc/diskstats race:** Reading `/proc/diskstats` twice with a short sleep to compute an I/O rate delta can give misleading results on nodes with high existing I/O load. The active write benchmark is more reliable than the delta approach for a pre-join check.

**anatol/smart.go and NVMe namespace vs controller device:** NVMe drives expose two device types — the controller (`/dev/nvme0`) and one or more namespaces (`/dev/nvme0n1`, `/dev/nvme0n2`). SMART admin commands must be issued to the controller device. `ghw.Block()` returns namespace devices for I/O purposes. Enumerate `/dev/nvme*` separately, filtering to controller-only entries (those matching `/dev/nvmeN` where N is a single integer with no trailing `n`).

**anatol/smart.go and hardware RAID:** Dell PERC, HP SmartArray, and LSI MegaRAID controllers present a single virtual `/dev/sdX` to the OS. The underlying physical drives are invisible to the ioctl passthrough approach. The library will either return an error or return data from the controller rather than the drives. Always catch the error from `smart.Open()` and return SKIP with a message indicating possible RAID controller — do not treat this as FAIL.

**SMART attribute 190 vs 194 temperature ambiguity:** Some drives report temperature in attribute 190 ("Airflow Temperature"), others in 194 ("Temperature Celsius"), and some report both. When both are present, attribute 194 is the composite drive temperature; attribute 190 is the airflow temperature entering the drive bay (always lower). Use 194 when present, fall back to 190, never sum or average them.

**Vendor-defined attributes above ID 128:** Do not attempt to interpret ATA attributes with IDs not in the explicit list in the implementation requirements above. Record their raw values in `Metadata["smart_raw_attr_<id>"]` as INFO — do not emit WARN or FAIL based on unknown attribute IDs, as the interpretation varies by drive manufacturer and model.

---

## What This Tool Must Never Do

- Modify any file outside a designated temp directory
- Make any changes to Nomad, Tailscale, or OS configuration
- Attempt to fix failing checks
- Infer jurisdiction from network location
- Cache results between runs
- Daemonise itself
- Require internet access (all checks are local or intra-network)
