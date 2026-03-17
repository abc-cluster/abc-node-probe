//go:build !linux

package probe

import "time"

func smartChecks(cfg Config) []checkFunc {
	return []checkFunc{
		func() CheckResult {
			return CheckResult{
				ID:         "storage.smart.devices",
				Category:   "storage",
				Name:       "S.M.A.R.T. Devices",
				Severity:   SeveritySkip,
				Message:    "S.M.A.R.T. checks are only available on Linux",
				DurationMs: time.Since(time.Now()).Milliseconds(),
			}
		},
	}
}
