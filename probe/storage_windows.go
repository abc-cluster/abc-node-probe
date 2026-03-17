//go:build windows

package probe

import "time"

func checkUlimits() CheckResult {
	return CheckResult{
		ID:         "storage.ulimits.open_files",
		Category:   "storage",
		Name:       "Open Files Ulimit",
		Severity:   SeveritySkip,
		Message:    "ulimit checks are not applicable on Windows",
		DurationMs: time.Since(time.Now()).Milliseconds(),
	}
}
