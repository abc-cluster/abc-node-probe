//go:build linux || darwin

package probe

import (
	"fmt"
	"syscall"
	"time"
)

func checkUlimits() CheckResult {
	start := time.Now()
	id := "storage.ulimits.open_files"
	cat := "storage"

	var rlimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "Open Files Ulimit",
			Severity: SeverityWarn, Message: fmt.Sprintf("could not read open files limit: %v", err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	sev := SeverityPass
	msg := ""
	if rlimit.Cur < 65536 {
		sev = SeverityWarn
		msg = fmt.Sprintf("open files soft limit=%d is low; recommend >= 65536", rlimit.Cur)
	}

	return CheckResult{
		ID: id, Category: cat, Name: "Open Files Ulimit",
		Severity: sev, Message: msg, Value: rlimit.Cur, Unit: "files",
		Metadata: map[string]string{"hard_limit": fmt.Sprintf("%d", rlimit.Max)},
		DurationMs: time.Since(start).Milliseconds(),
	}
}
