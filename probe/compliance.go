package probe

import (
	"fmt"
	"strings"
	"time"

	"github.com/abc-cluster/abc-node-probe/internal"
)

func complianceChecks(cfg Config) []checkFunc {
	return []checkFunc{
		func() CheckResult { return checkJurisdiction(cfg.Jurisdiction) },
		checkEncryptionAtRest,
		checkCrossBorderMounts,
	}
}

func checkJurisdiction(jurisdiction string) CheckResult {
	start := time.Now()
	id := "compliance.jurisdiction.declared"
	cat := "compliance"

	if jurisdiction == "" {
		return CheckResult{
			ID: id, Category: cat, Name: "Jurisdiction Declared",
			Severity: SeverityFail,
			Message:  "jurisdiction not declared — use --jurisdiction=<ISO-3166-1-alpha-2> flag",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	return CheckResult{
		ID: id, Category: cat, Name: "Jurisdiction Declared",
		Severity: SeverityPass,
		Message:  "Jurisdiction declared by operator. Value not independently verified.",
		Value:    strings.ToUpper(jurisdiction),
		Metadata: map[string]string{
			"declared_by": "operator_flag",
		},
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkEncryptionAtRest() CheckResult {
	start := time.Now()
	id := "compliance.encryption.data_at_rest"
	cat := "compliance"

	if !internal.IsRoot() {
		return CheckResult{
			ID: id, Category: cat, Name: "Encryption at Rest",
			Severity: SeveritySkip, Message: "requires root — skipping",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	if !internal.CommandExists("dmsetup") {
		return CheckResult{
			ID: id, Category: cat, Name: "Encryption at Rest",
			Severity: SeverityInfo, Message: "dmsetup not available — cannot verify disk encryption",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	out, err := internal.RunCommand("dmsetup", "info")
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "Encryption at Rest",
			Severity: SeverityInfo, Message: "no device mapper devices found",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	if strings.Contains(out, "LUKS") || strings.Contains(out, "crypt") {
		return CheckResult{
			ID: id, Category: cat, Name: "Encryption at Rest",
			Severity: SeverityPass, Message: "LUKS/dm-crypt encrypted device(s) detected",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	return CheckResult{
		ID: id, Category: cat, Name: "Encryption at Rest",
		Severity: SeverityWarn, Message: "no LUKS/dm-crypt encrypted devices detected — data at rest may not be encrypted",
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkCrossBorderMounts() CheckResult {
	start := time.Now()
	id := "compliance.mounts.cross_border"
	cat := "compliance"

	lines, err := internal.ReadProcFileLines("/proc/mounts")
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "Cross-Border Mounts",
			Severity: SeverityWarn, Message: "could not read /proc/mounts",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	var remoteMounts []string
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		fstype := fields[2]
		if fstype == "nfs" || fstype == "nfs4" || fstype == "cifs" || fstype == "smbfs" {
			remoteMounts = append(remoteMounts, fmt.Sprintf("%s(%s)", fields[1], fields[0]))
		}
	}

	if len(remoteMounts) > 0 {
		return CheckResult{
			ID: id, Category: cat, Name: "Cross-Border Mounts",
			Severity: SeverityInfo,
			Message: fmt.Sprintf("remote filesystem mounts detected — verify compliance with jurisdiction data residency requirements: %s",
				strings.Join(remoteMounts, ", ")),
			Value:      len(remoteMounts),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	return CheckResult{
		ID: id, Category: cat, Name: "Cross-Border Mounts",
		Severity: SeverityPass, Message: "no remote filesystem mounts detected",
		DurationMs: time.Since(start).Milliseconds(),
	}
}
