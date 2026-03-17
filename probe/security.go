package probe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/abc-cluster/abc-node-probe/internal"
)

func securityChecks(cfg Config) []checkFunc {
	return []checkFunc{
		checkSSHRootLogin,
		checkFirewallActive,
		checkWorldWritableDirs,
		checkTLSCertExpiry,
	}
}

func checkSSHRootLogin() CheckResult {
	start := time.Now()
	id := "security.ssh.root_login_disabled"
	cat := "security"

	// Try both common sshd config paths
	paths := []string{"/etc/ssh/sshd_config", "/etc/sshd_config"}
	var configPath string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			configPath = p
			break
		}
	}

	if configPath == "" {
		return CheckResult{
			ID: id, Category: cat, Name: "SSH Root Login Disabled",
			Severity: SeveritySkip, Message: "sshd_config not found — skipping",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "SSH Root Login Disabled",
			Severity: SeverityWarn, Message: fmt.Sprintf("could not read %s: %v", configPath, err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	// Parse PermitRootLogin setting
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if strings.EqualFold(fields[0], "PermitRootLogin") {
			val := strings.ToLower(fields[1])
			if val == "no" || val == "prohibit-password" || val == "forced-commands-only" {
				return CheckResult{
					ID: id, Category: cat, Name: "SSH Root Login Disabled",
					Severity: SeverityPass, Message: fmt.Sprintf("PermitRootLogin=%s", fields[1]),
					Metadata: map[string]string{"value": fields[1], "config": configPath},
					DurationMs: time.Since(start).Milliseconds(),
				}
			}
			return CheckResult{
				ID: id, Category: cat, Name: "SSH Root Login Disabled",
				Severity: SeverityFail, Message: fmt.Sprintf("PermitRootLogin=%s — direct root login is permitted", fields[1]),
				Metadata: map[string]string{"value": fields[1], "config": configPath},
				DurationMs: time.Since(start).Milliseconds(),
			}
		}
	}

	// Default behaviour is to allow root login if not explicitly set
	return CheckResult{
		ID: id, Category: cat, Name: "SSH Root Login Disabled",
		Severity: SeverityWarn, Message: "PermitRootLogin not explicitly set in sshd_config — default may allow root login",
		Metadata: map[string]string{"config": configPath},
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkFirewallActive() CheckResult {
	start := time.Now()
	id := "security.firewall.active"
	cat := "security"

	// Check for nftables, iptables, ufw, or firewalld
	checks := []struct {
		name    string
		command []string
	}{
		{"ufw", []string{"ufw", "status"}},
		{"firewalld", []string{"firewall-cmd", "--state"}},
		{"nftables", []string{"nft", "list", "ruleset"}},
	}

	for _, c := range checks {
		if !internal.CommandExists(c.command[0]) {
			continue
		}
		out, err := internal.RunCommand(c.command[0], c.command[1:]...)
		if err != nil {
			continue
		}
		out = strings.TrimSpace(out)
		if c.name == "ufw" && strings.Contains(out, "Status: active") {
			return CheckResult{
				ID: id, Category: cat, Name: "Firewall Active",
				Severity: SeverityPass, Message: "ufw firewall is active",
				Metadata: map[string]string{"firewall": "ufw"},
				DurationMs: time.Since(start).Milliseconds(),
			}
		}
		if c.name == "firewalld" && out == "running" {
			return CheckResult{
				ID: id, Category: cat, Name: "Firewall Active",
				Severity: SeverityPass, Message: "firewalld is running",
				Metadata: map[string]string{"firewall": "firewalld"},
				DurationMs: time.Since(start).Milliseconds(),
			}
		}
		if c.name == "nftables" && len(out) > 0 {
			return CheckResult{
				ID: id, Category: cat, Name: "Firewall Active",
				Severity: SeverityPass, Message: "nftables ruleset is loaded",
				Metadata: map[string]string{"firewall": "nftables"},
				DurationMs: time.Since(start).Milliseconds(),
			}
		}
	}

	// Check iptables as fallback
	if internal.CommandExists("iptables") {
		out, err := internal.RunCommand("iptables", "-L", "-n")
		if err == nil && strings.Contains(out, "Chain") {
			lines := strings.Split(out, "\n")
			ruleCount := 0
			for _, l := range lines {
				if strings.HasPrefix(l, "-") {
					ruleCount++
				}
			}
			if ruleCount > 0 {
				return CheckResult{
					ID: id, Category: cat, Name: "Firewall Active",
					Severity: SeverityPass, Message: fmt.Sprintf("iptables has %d rules active", ruleCount),
					Metadata: map[string]string{"firewall": "iptables"},
					DurationMs: time.Since(start).Milliseconds(),
				}
			}
		}
	}

	return CheckResult{
		ID: id, Category: cat, Name: "Firewall Active",
		Severity: SeverityWarn, Message: "no active firewall detected",
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkWorldWritableDirs() CheckResult {
	start := time.Now()
	id := "security.filesystem.world_writable_dirs"
	cat := "security"

	// Check a set of critical directories for world-writable permissions
	criticalDirs := []string{"/etc", "/usr", "/bin", "/sbin", "/lib", "/lib64"}
	var wwDirs []string

	for _, dir := range criticalDirs {
		info, err := os.Stat(dir)
		if err != nil {
			continue
		}
		mode := info.Mode()
		// World-writable: bit 0o002
		if mode&0o002 != 0 {
			wwDirs = append(wwDirs, fmt.Sprintf("%s(%s)", dir, mode.String()))
		}
	}

	if len(wwDirs) > 0 {
		return CheckResult{
			ID: id, Category: cat, Name: "World-Writable System Directories",
			Severity: SeverityFail,
			Message:  fmt.Sprintf("world-writable system directories detected: %s", strings.Join(wwDirs, ", ")),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	// Also check for world-writable files in /etc/cron.d if it exists
	cronFiles, _ := filepath.Glob("/etc/cron.d/*")
	var wwCron []string
	for _, f := range cronFiles {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if info.Mode()&0o002 != 0 {
			wwCron = append(wwCron, f)
		}
	}

	if len(wwCron) > 0 {
		return CheckResult{
			ID: id, Category: cat, Name: "World-Writable System Directories",
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("world-writable cron files: %s", strings.Join(wwCron, ", ")),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	return CheckResult{
		ID: id, Category: cat, Name: "World-Writable System Directories",
		Severity: SeverityPass, Message: "no world-writable system directories detected",
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkTLSCertExpiry() CheckResult {
	start := time.Now()
	id := "security.tls.cert_expiry"
	cat := "security"

	// Check Nomad TLS cert if present
	certPaths := []string{
		"/etc/nomad.d/certs/node.crt",
		"/etc/ssl/nomad/node.crt",
		"/opt/nomad/tls/node.crt",
	}

	for _, certPath := range certPaths {
		if _, err := os.Stat(certPath); err == nil {
			// Certificate exists — record it as INFO
			return CheckResult{
				ID: id, Category: cat, Name: "TLS Certificate Expiry",
				Severity: SeverityInfo,
				Message:  fmt.Sprintf("TLS certificate found at %s — manual expiry verification recommended", certPath),
				Metadata: map[string]string{"cert_path": certPath},
				DurationMs: time.Since(start).Milliseconds(),
			}
		}
	}

	return CheckResult{
		ID: id, Category: cat, Name: "TLS Certificate Expiry",
		Severity: SeveritySkip, Message: "no Nomad TLS certificates found at standard paths — skipping",
		DurationMs: time.Since(start).Milliseconds(),
	}
}
