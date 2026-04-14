# Nomad-Mode Implementation

## Overview

Added clean and intuitive exit behavior for abc-node-probe when used as a Nomad job. The tool now supports a `--nomad-mode` flag that ensures the job always exits with code 0 (success) regardless of probe results, allowing Nomad to complete the task successfully while still conveying the actual node readiness verdict through the JSON output.

## Problem Solved

Previously, abc-node-probe would exit with non-zero codes (1 for warnings, 2 for failures) to indicate node readiness issues. When running as a Nomad job, these non-zero exit codes would cause Nomad to interpret the task as failed and potentially restart it. This is undesirable because:

1. The probe is **informational** about node readiness, not a failure condition
2. Nomad should allow the probe task to complete successfully even if the node has issues
3. The actual readiness verdict should be determined by parsing the JSON output, not the exit code

## Solution

### For abc-node-probe

Added a `--nomad-mode` flag that:
- Always exits with code 0, indicating the probe task completed successfully
- Still collects and outputs all health check data
- Conveys node readiness verdict through:
  - JSON output: `summary.eligible_to_join` field (true/false)
  - Colored stdout output: summary section showing PASS/WARN/FAIL counts
  - API mode: posts results to control plane for centralized readiness decision

**Location:** `cmd/root.go`

**Implementation:**
```go
exitCode := exitCodeForReport(report)
// In nomad-mode, always exit 0 since the probe task completed successfully.
// Node readiness is conveyed via JSON output/API, not exit code.
if f.nomadMode {
    exitCode = 0
}
```

### For abc-cluster-cli

Updated the `abc infra compute probe` command to automatically enable `--nomad-mode` when dispatching probe jobs, since they always run in a Nomad context.

**Location:** `cmd/compute/probe.go`

**Change:** The probe job automatically includes `--nomad-mode` in its arguments:
```go
// Always use nomad-mode since we're running in a Nomad job context
probeArgs = append(probeArgs, "--nomad-mode")
```

## Exit Code Behavior

### Standard Mode (default)
- Exit 0: All checks PASS or SKIP
- Exit 1: At least one WARN (no FAILs)
- Exit 2: At least one FAIL
- Exit 3: Tool execution error

### Nomad Mode (`--nomad-mode`)
- Exit 0: Always (regardless of probe results)
- Exit 3: Tool execution error

## Usage Examples

### Direct probe invocation with nomad-mode
```bash
./abc-node-probe --nomad-mode --jurisdiction=ZA --json
```

### Nomad job with nomad-mode
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

### Via abc-cluster-cli (nomad-mode enabled automatically)
```bash
abc infra compute probe worker-01 --jurisdiction=ZA --json
```

### API mode with nomad-mode
```bash
./abc-node-probe --nomad-mode --mode=send \
  --jurisdiction=ZA \
  --api-endpoint=https://api.abc-cluster \
  --api-token=$TOKEN
```

## Determining Node Readiness

When using `--nomad-mode`, check node readiness via:

1. **JSON output parsing:**
   ```bash
   ./abc-node-probe --nomad-mode --json --jurisdiction=ZA | jq '.summary.eligible_to_join'
   ```

2. **Allocation logs (Nomad):**
   ```bash
   nomad alloc logs <alloc-id> probe | jq '.summary.eligible_to_join'
   ```

3. **API response (if using `--mode=send`):**
   ```json
   {
     "summary": {
       "eligible_to_join": true,
       "total_checks": 30,
       "pass_count": 27,
       "warn_count": 2,
       "fail_count": 1
     }
   }
   ```

## Documentation Updates

### README.md
- Added `--nomad-mode` to exit codes table
- Added "Using with Nomad" section with examples
- Updated command examples to include nomad-mode usage
- Clarified exit behavior in Nomad context

### GitHub Actions Workflows
- CI/build workflows remain compatible with nomad-mode
- Release artifacts work seamlessly with nomad-mode

## Backward Compatibility

- Default behavior unchanged: `--nomad-mode` is opt-in
- Existing scripts and direct invocations work as before
- Only affects behavior when `--nomad-mode` flag is explicitly used

## Testing

### Verification Steps
1. ✅ `./abc-node-probe --help` shows `--nomad-mode` flag
2. ✅ `./abc-node-probe --nomad-mode` exits with code 0
3. ✅ `./abc-node-probe --nomad-mode --json` outputs valid JSON with readiness verdict
4. ✅ `abc infra compute probe <node>` uses nomad-mode automatically
5. ✅ Nomad job exits cleanly even if probe reports failures

## Future Enhancements

Potential improvements:

1. **Environment variable support:** Allow `ABC_PROBE_NOMAD_MODE=1` to enable nomad-mode
2. **Automatic detection:** Detect Nomad environment and auto-enable nomad-mode
3. **Health check integration:** Support Nomad health checks that query the JSON output
4. **Metrics export:** Export probe metrics to Nomad metrics systems

---

**Files Modified:**
- `cmd/root.go` — Added nomad-mode flag and exit code logic
- `cmd/compute/probe.go` — Auto-enable nomad-mode for Nomad job dispatch
- `README.md` — Updated documentation with usage examples and clarifications
- `.gitignore` — Updated build artifact patterns

**Files Created:**
- `.github/workflows/ci.yml` — CI pipeline with test/lint/build
- `.github/workflows/build-release.yml` — Release build and packaging
- `.github/workflows/vulnerability-check.yml` — Security scanning
- `.github/WORKFLOWS.md` — Workflow documentation
- `probe/speedtest.go` — Network speedtest check implementation
- `probe/speedtest_test.go` — Speedtest tests
