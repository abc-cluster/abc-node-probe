package probe

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/abc-cluster/abc-node-probe/internal"
)

func networkChecks(cfg Config) []checkFunc {
	return []checkFunc{
		checkTailscaleDaemon,
		checkNTPSync,
		checkDNSResolution,
		checkNetworkInterfaces,
	}
}

// tailscaleStatus is a minimal struct for parsing `tailscale status --json` output.
type tailscaleStatus struct {
	BackendState string `json:"BackendState"`
	Self         struct {
		Online bool `json:"Online"`
	} `json:"Self"`
}

func checkTailscaleDaemon() CheckResult {
	start := time.Now()
	id := "network.tailscale.daemon_running"
	cat := "network"

	if !internal.CommandExists("tailscale") {
		return CheckResult{
			ID: id, Category: cat, Name: "Tailscale Daemon Running",
			Severity: SeverityWarn, Message: "tailscale binary not found in PATH — skipping",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	out, err := internal.RunCommand("tailscale", "status", "--json")
	if err != nil && out == "" {
		return CheckResult{
			ID: id, Category: cat, Name: "Tailscale Daemon Running",
			Severity: SeverityWarn, Message: fmt.Sprintf("tailscale daemon not running or not accessible: %v", err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	var status tailscaleStatus
	if jsonErr := json.Unmarshal([]byte(out), &status); jsonErr != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "Tailscale Daemon Running",
			Severity: SeverityWarn, Message: fmt.Sprintf("could not parse tailscale status: %v", jsonErr),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	if status.BackendState != "Running" {
		return CheckResult{
			ID: id, Category: cat, Name: "Tailscale Daemon Running",
			Severity: SeverityFail, Message: fmt.Sprintf("Tailscale backend state: %s (expected Running)", status.BackendState),
			Metadata: map[string]string{"backend_state": status.BackendState},
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	return CheckResult{
		ID: id, Category: cat, Name: "Tailscale Daemon Running",
		Severity: SeverityPass, Message: "Tailscale daemon is running",
		Metadata: map[string]string{"backend_state": status.BackendState},
		DurationMs: time.Since(start).Milliseconds(),
	}
}

// parseTimedatectl parses key=value pairs from `timedatectl show` output.
func parseTimedatectl(out string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.IndexByte(line, '='); idx > 0 {
			result[line[:idx]] = line[idx+1:]
		}
	}
	return result
}

func checkNTPSync() CheckResult {
	start := time.Now()
	id := "network.ntp.sync_status"
	cat := "network"

	if !internal.CommandExists("timedatectl") {
		return CheckResult{
			ID: id, Category: cat, Name: "NTP Sync Status",
			Severity: SeveritySkip, Message: "timedatectl not found — non-systemd system, skipping NTP check",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	out, err := internal.RunCommand("timedatectl", "show")
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "NTP Sync Status",
			Severity: SeverityWarn, Message: fmt.Sprintf("timedatectl failed: %v", err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	fields := parseTimedatectl(out)
	syncedStr := fields["NTPSynchronized"]
	if syncedStr == "" {
		syncedStr = fields["NetworkTimeOn"]
	}

	if strings.ToLower(syncedStr) != "yes" {
		return CheckResult{
			ID: id, Category: cat, Name: "NTP Sync Status",
			Severity: SeverityFail, Message: "NTP not synchronized",
			Metadata: map[string]string{"ntp_synchronized": syncedStr},
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	return CheckResult{
		ID: id, Category: cat, Name: "NTP Sync Status",
		Severity: SeverityPass, Message: "NTP is synchronized",
		Metadata: map[string]string{"ntp_synchronized": syncedStr},
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkDNSResolution() CheckResult {
	start := time.Now()
	id := "network.dns.resolution"
	cat := "network"

	testHost := "localhost"
	_, err := net.LookupHost(testHost)
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "DNS Resolution",
			Severity: SeverityFail, Message: fmt.Sprintf("DNS resolution failed for %s: %v", testHost, err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	return CheckResult{
		ID: id, Category: cat, Name: "DNS Resolution",
		Severity: SeverityPass, Message: "DNS resolution working",
		DurationMs: time.Since(start).Milliseconds(),
	}
}
