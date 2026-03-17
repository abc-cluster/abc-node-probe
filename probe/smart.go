//go:build linux

package probe

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/anatol/smart.go"
	"github.com/jaypipes/ghw"

	"github.com/abc-cluster/abc-node-probe/internal"
)

func smartChecks(cfg Config) []checkFunc {
	start := time.Now()

	// Check permissions first
	if !internal.IsRoot() && !internal.InGroup("disk") {
		skipResult := CheckResult{
			ID:         "storage.smart.permission",
			Category:   "storage",
			Name:       "S.M.A.R.T. Permission",
			Severity:   SeveritySkip,
			Message:    "requires root or disk group membership",
			DurationMs: time.Since(start).Milliseconds(),
		}
		return []checkFunc{func() CheckResult { return skipResult }}
	}

	// Enumerate physical block devices
	devices := enumeratePhysicalDevices()
	nvmeDevs := enumerateNVMeControllers()

	if len(devices) == 0 && len(nvmeDevs) == 0 {
		skipResult := CheckResult{
			ID:         "storage.smart.devices",
			Category:   "storage",
			Name:       "S.M.A.R.T. Devices",
			Severity:   SeveritySkip,
			Message:    "no physical block devices detected",
			DurationMs: time.Since(start).Milliseconds(),
		}
		return []checkFunc{func() CheckResult { return skipResult }}
	}

	var checks []checkFunc
	for _, dev := range devices {
		dev := dev
		checks = append(checks, func() CheckResult {
			return checkSmartDevice(dev)
		})
	}
	for _, dev := range nvmeDevs {
		dev := dev
		checks = append(checks, func() CheckResult {
			return checkSmartDevice(dev)
		})
	}

	return checks
}

// partitionPattern matches device names ending in a digit after a non-digit (e.g. sda1)
var partitionPattern = regexp.MustCompile(`[a-z]\d+$`)

// nvmeNamespacePattern matches nvme namespace devices like nvme0n1
var nvmeNamespacePattern = regexp.MustCompile(`^nvme\d+n\d+`)

// nvmeControllerPattern matches nvme controller devices like nvme0
var nvmeControllerPattern = regexp.MustCompile(`^nvme\d+$`)

func enumeratePhysicalDevices() []string {
	blk, err := ghw.Block()
	if err != nil {
		return nil
	}

	var devices []string
	for _, disk := range blk.Disks {
		name := disk.Name
		devPath := "/dev/" + name

		// Filter out virtual/partition/nvme devices (nvme handled separately)
		if strings.HasPrefix(name, "loop") ||
			strings.HasPrefix(name, "dm-") ||
			strings.HasPrefix(name, "ram") ||
			nvmeNamespacePattern.MatchString(name) ||
			nvmeControllerPattern.MatchString(name) ||
			partitionPattern.MatchString(name) {
			continue
		}

		devices = append(devices, devPath)
	}
	return devices
}

func enumerateNVMeControllers() []string {
	// Enumerate /dev/nvmeN controller devices (not namespaces)
	matches, err := filepath.Glob("/dev/nvme[0-9]")
	if err != nil {
		return nil
	}
	more, _ := filepath.Glob("/dev/nvme[0-9][0-9]")
	matches = append(matches, more...)
	return matches
}

func devShortName(devPath string) string {
	return strings.TrimPrefix(devPath, "/dev/")
}

func checkSmartDevice(devPath string) CheckResult {
	shortName := devShortName(devPath)
	baseID := fmt.Sprintf("storage.smart.%s", shortName)

	dev, err := smart.Open(devPath)
	if err != nil {
		sev := SeveritySkip
		msg := fmt.Sprintf("device does not support SMART or passthrough unavailable (possible hardware RAID controller): %v", err)
		if strings.Contains(err.Error(), "permission denied") {
			msg = fmt.Sprintf("permission denied opening %s — skipping", devPath)
		} else if strings.Contains(err.Error(), "no such device") || strings.Contains(err.Error(), "no such file") {
			msg = fmt.Sprintf("device %s not found — skipping", devPath)
		}
		return CheckResult{
			ID:         baseID + ".overall_health",
			Category:   "storage",
			Name:       fmt.Sprintf("S.M.A.R.T. %s Overall Health", shortName),
			Severity:   sev,
			Message:    msg,
			Metadata:   map[string]string{"device": devPath},
			DurationMs: 0,
		}
	}
	defer dev.Close()

	switch d := dev.(type) {
	case *smart.SataDevice:
		return checkATADevice(d, devPath, baseID)
	case *smart.NVMeDevice:
		return checkNVMeDevice(d, devPath, baseID)
	case *smart.ScsiDevice:
		return checkSCSIDevice(devPath, baseID)
	default:
		return CheckResult{
			ID:       baseID + ".overall_health",
			Category: "storage",
			Name:     fmt.Sprintf("S.M.A.R.T. %s Overall Health", shortName),
			Severity: SeveritySkip,
			Message:  "unrecognised drive interface — skipping to avoid misinterpretation",
			Metadata: map[string]string{"device": devPath},
		}
	}
}

func checkATADevice(dev *smart.SataDevice, devPath, baseID string) CheckResult {
	start := time.Now()
	shortName := devShortName(devPath)

	attrs, err := dev.ReadSMARTData()
	if err != nil {
		return CheckResult{
			ID:         baseID + ".overall_health",
			Category:   "storage",
			Name:       fmt.Sprintf("S.M.A.R.T. %s Overall Health", shortName),
			Severity:   SeveritySkip,
			Message:    fmt.Sprintf("could not read SMART data: %v", err),
			Metadata:   map[string]string{"device": devPath, "type": "ata"},
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	meta := map[string]string{
		"device": devPath,
		"type":   "ata",
	}
	if id, err := dev.Identify(); err == nil {
		meta["model"] = strings.TrimSpace(id.ModelNumber())
		meta["serial"] = strings.TrimSpace(id.SerialNumber())
	}

	sev := SeverityPass
	msg := "drive health check passed"

	for _, attr := range attrs.Attrs {
		raw := attr.ValueRaw
		switch attr.Id {
		case 5: // Reallocated Sector Count
			if raw > 10 {
				sev = SeverityFail
				msg = fmt.Sprintf("reallocated sector count=%d (critical)", raw)
			} else if raw > 0 && sev != SeverityFail {
				sev = SeverityWarn
				msg = fmt.Sprintf("reallocated sector count=%d", raw)
			}
		case 187: // Reported Uncorrectable Errors
			if raw > 0 {
				sev = SeverityFail
				msg = fmt.Sprintf("reported uncorrectable errors=%d", raw)
			}
		case 198: // Uncorrectable Sector Count
			if raw > 0 {
				sev = SeverityFail
				msg = fmt.Sprintf("uncorrectable sector count=%d — data loss confirmed at physical level", raw)
			}
		}
	}

	return CheckResult{
		ID:         baseID + ".overall_health",
		Category:   "storage",
		Name:       fmt.Sprintf("S.M.A.R.T. %s Overall Health", shortName),
		Severity:   sev,
		Message:    msg,
		Metadata:   meta,
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkNVMeDevice(dev *smart.NVMeDevice, devPath, baseID string) CheckResult {
	start := time.Now()
	shortName := devShortName(devPath)

	log, err := dev.ReadSMART()
	if err != nil {
		return CheckResult{
			ID:         baseID + ".overall_health",
			Category:   "storage",
			Name:       fmt.Sprintf("S.M.A.R.T. %s Overall Health", shortName),
			Severity:   SeveritySkip,
			Message:    fmt.Sprintf("could not read NVMe health log: %v", err),
			Metadata:   map[string]string{"device": devPath, "type": "nvme"},
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	meta := map[string]string{
		"device": devPath,
		"type":   "nvme",
	}

	if id, _, err := dev.Identify(); err == nil {
		meta["model"] = strings.TrimSpace(string(id.ModelNumberRaw[:]))
		meta["serial"] = strings.TrimSpace(string(id.SerialNumberRaw[:]))
	}

	sev := SeverityPass
	msg := "NVMe health check passed"

	// CritWarning bitmask
	if log.CritWarning != 0 {
		sev = SeverityFail
		msg = fmt.Sprintf("NVMe critical warning bitmask=0x%02x", log.CritWarning)
	}

	// MediaErrors (Uint128 — check Lo field)
	if log.MediaErrors.Val[0] > 0 || log.MediaErrors.Val[1] > 0 {
		sev = SeverityFail
		msg = fmt.Sprintf("NVMe media errors detected")
	}

	// AvailSpare
	spare := log.AvailSpare
	if spare < 10 {
		sev = SeverityFail
		msg = fmt.Sprintf("available spare=%d%% — below 10%% threshold", spare)
	}

	// PercentUsed
	pctUsed := log.PercentUsed
	if pctUsed > 99 {
		sev = SeverityFail
		msg = fmt.Sprintf("percentage used=%d%% — drive worn out", pctUsed)
	} else if pctUsed > 90 && sev == SeverityPass {
		sev = SeverityWarn
		msg = fmt.Sprintf("percentage used=%d%%", pctUsed)
	}

	// Temperature (Kelvin → Celsius)
	tempC := int(log.Temperature) - 273
	if tempC > 70 && sev == SeverityPass {
		sev = SeverityFail
		msg = fmt.Sprintf("NVMe temperature=%d°C — critical", tempC)
	} else if tempC > 60 && sev == SeverityPass {
		sev = SeverityWarn
		msg = fmt.Sprintf("NVMe temperature=%d°C — elevated", tempC)
	}

	meta["temperature_c"] = fmt.Sprintf("%d", tempC)
	meta["available_spare_pct"] = fmt.Sprintf("%d", spare)
	meta["percentage_used"] = fmt.Sprintf("%d", pctUsed)

	return CheckResult{
		ID:         baseID + ".overall_health",
		Category:   "storage",
		Name:       fmt.Sprintf("S.M.A.R.T. %s Overall Health", shortName),
		Severity:   sev,
		Message:    msg,
		Metadata:   meta,
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkSCSIDevice(devPath, baseID string) CheckResult {
	start := time.Now()
	shortName := devShortName(devPath)

	return CheckResult{
		ID:       baseID + ".overall_health",
		Category: "storage",
		Name:     fmt.Sprintf("S.M.A.R.T. %s Overall Health", shortName),
		Severity: SeverityInfo,
		Message:  "SCSI/SAS device detected — limited SMART data available",
		Metadata: map[string]string{"device": devPath, "type": "scsi"},
		DurationMs: time.Since(start).Milliseconds(),
	}
}
