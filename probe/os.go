package probe

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"

	"github.com/abc-cluster/abc-node-probe/internal"
)

func osChecks(cfg Config) []checkFunc {
	return []checkFunc{
		checkKernelVersion,
		checkCgroupsVersion,
		checkNamespaces,
		checkSystemdRunning,
		checkSELinuxStatus,
		checkOverlayFSSupport,
	}
}

func checkKernelVersion() CheckResult {
	start := time.Now()
	id := "os.kernel.version"
	cat := "os"

	version, err := internal.KernelVersion()
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "Kernel Version",
			Severity: SeverityWarn, Message: "could not read kernel version",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	return CheckResult{
		ID: id, Category: cat, Name: "Kernel Version",
		Severity: SeverityInfo, Value: version,
		Message: "kernel version recorded",
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkCgroupsVersion() CheckResult {
	start := time.Now()
	id := "os.cgroups.version"
	cat := "os"

	// Check if cgroup2 is mounted (unified hierarchy)
	content, err := os.ReadFile("/proc/filesystems")
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "cgroups Version",
			Severity: SeverityWarn, Message: "could not read /proc/filesystems",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	hasCgroup2 := strings.Contains(string(content), "cgroup2")

	// Check what's actually mounted
	mountContent, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "cgroups Version",
			Severity: SeverityWarn, Message: "could not read /proc/mounts",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	cgroupV2Mounted := strings.Contains(string(mountContent), "cgroup2")

	version := "v1"
	if hasCgroup2 && cgroupV2Mounted {
		version = "v2"
	} else if hasCgroup2 {
		version = "v1+v2-available"
	}

	sev := SeverityPass
	msg := fmt.Sprintf("cgroups %s active", version)
	if version == "v1" {
		sev = SeverityWarn
		msg = "cgroups v1 only — Nomad works but v2 preferred"
	}

	return CheckResult{
		ID: id, Category: cat, Name: "cgroups Version",
		Severity: sev, Message: msg, Value: version,
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkNamespaces() CheckResult {
	start := time.Now()
	id := "os.kernel.namespaces_available"
	cat := "os"

	// Check for key namespace types required by container workloads
	requiredNamespaces := []string{"mnt", "uts", "ipc", "pid", "net", "user"}
	var missing []string

	for _, ns := range requiredNamespaces {
		path := fmt.Sprintf("/proc/self/ns/%s", ns)
		if _, err := os.Lstat(path); err != nil {
			missing = append(missing, ns)
		}
	}

	if len(missing) > 0 {
		return CheckResult{
			ID: id, Category: cat, Name: "Kernel Namespaces",
			Severity: SeverityFail,
			Message:  fmt.Sprintf("missing kernel namespaces: %s", strings.Join(missing, ", ")),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	return CheckResult{
		ID: id, Category: cat, Name: "Kernel Namespaces",
		Severity: SeverityPass,
		Message:  fmt.Sprintf("all required namespaces available: %s", strings.Join(requiredNamespaces, ", ")),
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkSystemdRunning() CheckResult {
	start := time.Now()
	id := "os.systemd.running"
	cat := "os"

	// Check if systemd is PID 1 via /proc/1/comm
	comm, err := internal.ReadProcFile("/proc/1/comm")
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "Systemd Running",
			Severity: SeveritySkip, Message: "could not read /proc/1/comm",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	if !strings.Contains(comm, "systemd") {
		return CheckResult{
			ID: id, Category: cat, Name: "Systemd Running",
			Severity: SeveritySkip, Message: fmt.Sprintf("PID 1 is %q — not systemd, skipping systemd checks", comm),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	conn, err := dbus.NewSystemConnection()
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "Systemd Running",
			Severity: SeverityWarn, Message: fmt.Sprintf("systemd dbus connection failed: %v", err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}
	defer conn.Close()

	return CheckResult{
		ID: id, Category: cat, Name: "Systemd Running",
		Severity: SeverityPass, Message: "systemd is running as PID 1 and DBus is accessible",
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkSELinuxStatus() CheckResult {
	start := time.Now()
	id := "os.selinux.status"
	cat := "os"

	content, err := os.ReadFile("/sys/fs/selinux/enforce")
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "SELinux Status",
			Severity: SeverityInfo, Message: "SELinux not present or not mounted",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	val := strings.TrimSpace(string(content))
	switch val {
	case "1":
		return CheckResult{
			ID: id, Category: cat, Name: "SELinux Status",
			Severity: SeverityWarn, Message: "SELinux is in enforcing mode — container workloads may require policy adjustment",
			Value: "enforcing",
			DurationMs: time.Since(start).Milliseconds(),
		}
	case "0":
		return CheckResult{
			ID: id, Category: cat, Name: "SELinux Status",
			Severity: SeverityInfo, Message: "SELinux is in permissive mode",
			Value: "permissive",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	return CheckResult{
		ID: id, Category: cat, Name: "SELinux Status",
		Severity: SeverityInfo, Message: fmt.Sprintf("SELinux enforce file contains: %q", val),
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkOverlayFSSupport() CheckResult {
	start := time.Now()
	id := "os.kernel.overlayfs"
	cat := "os"

	content, err := os.ReadFile("/proc/filesystems")
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "OverlayFS Support",
			Severity: SeverityWarn, Message: "could not read /proc/filesystems",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	if strings.Contains(string(content), "overlay") {
		return CheckResult{
			ID: id, Category: cat, Name: "OverlayFS Support",
			Severity: SeverityPass, Message: "overlay filesystem is supported",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	return CheckResult{
		ID: id, Category: cat, Name: "OverlayFS Support",
		Severity: SeverityWarn, Message: "overlay filesystem not found in /proc/filesystems — container layer stacking may not work",
		DurationMs: time.Since(start).Milliseconds(),
	}
}
