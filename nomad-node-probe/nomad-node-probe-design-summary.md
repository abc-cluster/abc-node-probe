# nomad-node-probe: Design Discussion Summary

## Context

This document summarises the design conversation for **nomad-node-probe**, a pre-join health check tool for the ABC-cluster sovereign hybrid compute orchestration platform. The tool assesses any prospective node (university server, HPC login node, cloud VM) before it is admitted into the ABC-cluster Tailscale tailnet and registered as a Nomad client.

---

## Purpose & Philosophy

The tool acts as a **pre-join gate** — run once on a prospective node before the admission controller processes its registration request. Its output feeds directly into Nomad node metadata tags for scheduler constraint matching, and is stored in MinIO as a permanent admission audit record.

### Severity Levels

| Level | Meaning |
|---|---|
| **PASS** | Requirement fully met |
| **WARN** | Node can join with restricted workload eligibility |
| **FAIL** | Hard blocker — node cannot join until resolved |
| **SKIP** | Check not applicable to this node type |

---

## Check Categories

### 1. Hardware Fingerprinting (Classification Only — no FAIL conditions)
- CPU architecture, physical core count, NUMA topology
- RAM total and available at check time
- Swap presence
- GPU detection via `/proc` or `lspci`

### 2. Storage Checks (Most Critical)
- Local scratch mount existence, free space (FAIL if < 50GB), filesystem type, sequential write IOPS
- MinIO endpoint reachability and credentials (both FAIL)
- JuiceFS and Lustre mount detection (SKIP where not applicable)
- Disk encryption status via LUKS detection (POPIA relevance)
- inotify limits and open file descriptor limits — frequently missed, cause mysterious Nextflow failures
- **S.M.A.R.T. drive health** — per-device physical health assessment (see S.M.A.R.T. section below)

### 3. Network Checks
- Tailscale binary, daemon, assigned IP (all FAIL if missing)
- Tailscale MTU, MagicDNS resolution
- Nomad server and admission controller reachability (FAIL)
- NTP sync status — **elevated to FAIL** due to TigerBeetle clock skew sensitivity and Nomad leader election requirements
- OCI Object Storage endpoint reachability
- Outbound port 443 for rclone

### 4. Operating System & Kernel Checks
- OS family, kernel version (WARN if < 4.4)
- Kernel namespace availability (FAIL — required for Nomad `exec` driver)
- cgroups v1 vs v2 detection
- SELinux/AppArmor status, systemd presence
- Clock source inspection via `/sys/devices/system/clocksource`

### 5. Nomad-Specific Checks
- Nomad binary presence and version compatibility (FAIL)
- Config file parseability (HCL validation)
- Task drivers available: `exec`, `java`, `docker`, `raw_exec`
- HPC bridge driver binary presence for PBS/SLURM nodes
- Nomad data directory writability

### 6. Nextflow / Bioinformatics Runtime Checks (Optional — compute nodes only)
- Java version (FAIL if < 11), JVM heap max
- rclone binary and version (FAIL — required for nf-rclone plugin)
- Singularity/Apptainer presence
- Conda/Mamba presence (INFO)

### 7. Data Sovereignty & Compliance Checks (Non-optional for POPIA/GDPR nodes)
- Physical jurisdiction declaration via explicit `--jurisdiction=ZA` flag — **deliberately not automated**; human declaration is auditable
- Nomad region matches declared jurisdiction (FAIL if mismatch)
- Audit log directory writability and OPA agent reachability (FAIL)
- No unauthorised cross-border NFS/CIFS mounts
- Data encryption at rest

### 8. Security Baseline Checks
- SSH root login disabled
- World-writable directories in data paths (FAIL)
- Nomad gossip encryption key configured (FAIL)
- Nomad TLS configuration
- Firewall presence detection

---

## Output Format

1. **Human-readable terminal output** — coloured PASS/WARN/FAIL with explanations
2. **JSON report** — machine-readable, ingested by admission controller or written to file
3. **Exit codes** — `0` all-pass, `1` any WARN, `2` any FAIL

---

## Language Selection

### Why Go

| Criterion | Assessment |
|---|---|
| Single static binary | `CGO_DISABLED=1` produces a fully static executable |
| Zero runtime prerequisites | No JRE, no Python interpreter, no shared libraries |
| Cross-compilation | Single command: `GOOS=linux GOARCH=amd64 go build` |
| `/proc` and `/sys` access | Direct file reads — stable across Ubuntu 14.04 → 24.04 |
| HCL config validation | HashiCorp publishes `github.com/hashicorp/hcl/v2` in Go |
| Existing codebase alignment | Matches HPC bridge driver and Nomad task driver language |

### Go Dependency Footprint (5–7 libraries total)

| Library | Purpose |
|---|---|
| `github.com/shirou/gopsutil` | CPU, memory, disk, network — cross-platform |
| `github.com/jaypipes/ghw` | NUMA topology, PCI/GPU enumeration, block device enumeration |
| `github.com/vishvananda/netlink` | MTU, route inspection via Linux netlink socket |
| `golang.org/x/sys` | Cgroups, kernel namespaces, low-level syscalls |
| `github.com/coreos/go-systemd` | Systemd unit status checks |
| `github.com/hashicorp/hcl/v2` | Nomad config file validation |
| `github.com/anatol/smart.go` | Pure Go S.M.A.R.T. ioctl passthrough — no subprocess, no smartmontools |

### Other Languages Evaluated

| Language | Distribution | Verdict |
|---|---|---|
| **Go** | Single static binary | **Primary choice** |
| Rust | Single static binary (musl) | Strong alternative; cross-compilation toolchain more complex |
| Python + PyInstaller | Bundled interpreter (~50MB) | Script-only variant for sysadmin convenience; not the authoritative binary |
| Bash | Universal | Companion quick-check script only; JSON output painful without `jq` |
| Java / Kotlin JVM | Requires JRE | Rejected — deployment problem is structural |
| Kotlin Native | Static binary | Ecosystem too immature for system-level checks |
| Zig | Static binary | Promising for future RISC-V HPC targets; not production-ready today |

---

## S.M.A.R.T. Disk Health Checks

### What S.M.A.R.T. Adds

The existing storage checks measure **current state** — available space, endpoint reachability, write throughput. S.M.A.R.T. adds a **historical and predictive** dimension: firmware-level telemetry maintained by the drive itself, covering physical degradation that filesystem-level checks cannot see.

### Implementation Approach

S.M.A.R.T. data is read via Linux ioctl passthrough — no subprocess, no smartmontools installation required. The library `github.com/anatol/smart.go` implements this in pure Go:

- SATA/ATA drives — `SG_IO` ioctl with `ATA PASS-THROUGH (16)` command
- NVMe drives — `NVME_IOCTL_ADMIN_CMD` ioctl, reading SMART/Health Information log page (0x02)
- SCSI/SAS drives — native `LOG SENSE` command via `SG_IO`

Drive enumeration uses `ghw.Block()` (already in the dependency list), filtering out virtual devices (loop, dm-*, RAM disks) before attempting SMART reads.

### Checks Implemented and Their Thresholds

**Overall Health Verdict** — the drive firmware's own pass/fail assessment. FAIL if the drive predicts imminent failure. This is the single highest-value SMART check.

**ATA Attribute Thresholds:**

| Attribute ID | Name | WARN | FAIL |
|---|---|---|---|
| 5 | Reallocated Sector Count | raw > 0 | raw > 10 |
| 9 | Power-On Hours | raw > 40,000 | — |
| 177 | Wear Leveling Count (SSD) | current < 10 | — |
| 187 | Reported Uncorrectable Errors | — | raw > 0 |
| 190 / 194 | Temperature (°C) | raw > 60 | raw > 70 |
| 197 | Current Pending Sector Count | raw > 0 | — |
| 198 | Uncorrectable Sector Count | — | raw > 0 |
| 231 | SSD Life Left (%) | raw < 10 | — |

**NVMe Health Log Fields:**

| Field | WARN | FAIL |
|---|---|---|
| CriticalWarning bitmask | — | any bit set |
| AvailableSpare (%) | — | < 10 |
| PercentageUsed | > 90 | > 99 |
| MediaErrors | — | > 0 |

### Applicability and Skip Conditions

SMART checks SKIP on:
- Nodes running as non-root without `disk` group membership
- Virtual block devices on cloud VMs (OCI paravirtualised devices have no SMART backend)
- Drives behind hardware RAID controllers where passthrough is unavailable — detected by catching device open errors, not by attempting to identify the controller

SMART checks are expected to SKIP on approximately 70–80% of target nodes (cloud VMs, HPC compute nodes without elevated permissions). The 20–30% where they run — dedicated bare-metal storage nodes, physical login nodes — are exactly where drive failure prediction has the highest operational value.

### Per-Device Result Structure

SMART results are per-device, not per-node. Each `CheckResult` carries in `Metadata`:

```
device  = /dev/sda
model   = WDC WD4000FYYZ-01UL1B1
serial  = WD-WCATR1234567
type    = ata | nvme | scsi
```

This allows the admission controller to surface which specific drive is at risk rather than a generic node-level warning.

### What smartmontools Has That anatol/smart.go Does Not

- 20 years of device-specific quirk handling for exotic or older drives
- A vendor database mapping drive model strings to correct attribute interpretations for vendor-defined attributes (IDs > 128)
- USB-attached drive support via SAT (SCSI-ATA Translation)

For nomad-node-probe, the mitigation is conservative: only standardised ATA attributes (IDs ≤ 128) and well-known vendor attributes (190, 194, 197, 198, 231) are interpreted. Drives where interpretation is uncertain return INFO rather than WARN or FAIL. A failed SMART read on an unrecognised device type always returns SKIP, never FAIL.

---



### What osquery Is

osquery exposes OS state as SQL-queryable virtual tables (`cpu_info`, `mounts`, `disk_encryption`, `ntp_peers`, `pci_devices`, `systemd_units`, etc.). It is battle-tested at scale, originally built by Meta for fleet-wide security monitoring.

**`osquery-go` is not an embedding library.** It is an extension SDK for writing Go plugins that run *inside* a running osquery daemon over a Thrift IPC channel. This is a common misconception from reading the documentation.

### Why osquery Is Incompatible with nomad-node-probe

- osqueryd is a C++ daemon that must be **pre-installed and running** on the target node
- It cannot be compiled into a Go binary — different language, different build system, no static embedding path
- `CGO_DISABLED=1` is incompatible with any CGo bridge to osquery's C++ internals
- The deployment model (daemon + socket + extension) is the structural opposite of a single self-contained pre-join executable

### Where osquery Belongs in ABC-cluster

osquery is the right tool for a **different, future component**: a post-join continuous compliance monitoring agent, deployed as a Nomad system job (one allocation per node), querying `disk_encryption`, `ntp_peers`, `iptables`, and `systemd_units` on a scheduled basis and writing results to MinIO as a POPIA audit artefact. This is the pattern used by fleet security platforms (Kolide, Fleet, Uptycs).

| Concern | nomad-node-probe | Future compliance agent |
|---|---|---|
| When it runs | Once, at pre-join | Continuously, post-join |
| Output destination | Admission controller | MinIO + dashboard |
| Static binary required | Yes | No |
| osquery suitable | **No** | **Yes** |

**Design note:** The admission controller JSON schema should be designed so the future osquery-based compliance agent can write into the same schema, preserving optionality without coupling the two tools prematurely.

---

## Deployment Pattern

```bash
curl -O https://abc-cluster-registry/tools/abc-probe-linux-amd64
chmod +x abc-probe-linux-amd64
./abc-probe-linux-amd64 \
  --jurisdiction=ZA \
  --node-role=compute \
  --nomad-server=100.x.x.x:4647 \
  --output=json \
  --send
```

- No package manager required
- Root not required for most checks (those that need it are clearly flagged)
- Runs in under 30 seconds on any reasonable hardware
- Kernel 3.x or newer supported

---

## What the Tool Is Not

- Not a continuous monitoring tool — that is Nomad's built-in health checks and the observability stack
- Not a configuration management tool — it reports findings, it does not fix them
- Not a network scanner — it checks only what the node itself can reach

> The right mental model: **this is the nurse's intake assessment before the doctor (admission controller) decides whether to admit the patient.**
