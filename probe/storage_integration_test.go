//go:build integration

package probe

import (
	"runtime"
	"testing"
)

func TestScratchIOPS_BenchmarkAgainstTmp(t *testing.T) {
	// checkScratchIOPS falls back to the OS temp dir on all platforms
	t.Logf("Running IOPS benchmark (platform: %s)", runtime.GOOS)

	result := checkScratchIOPS()
	if result.ID == "" {
		t.Error("expected non-empty result ID")
	}
	if result.Severity == SeverityFail {
		t.Logf("IOPS benchmark returned FAIL: %s — may indicate slow temp storage", result.Message)
	}
	if result.Value == nil {
		t.Error("expected a throughput value in the result")
	}
}

func TestScratchFreeSpace_ReturnsResult(t *testing.T) {
	result := checkScratchFreeSpace()
	if result.ID == "" {
		t.Errorf("checkScratchFreeSpace returned empty ID")
	}
}

func TestSmartChecks_GracefulOnAllPlatforms(t *testing.T) {
	cfg := Config{NodeRole: "compute"}
	checks := smartChecks(cfg)
	if len(checks) == 0 {
		t.Fatal("smartChecks returned no check functions")
	}
	for _, fn := range checks {
		result := fn()
		if result.ID == "" {
			t.Error("SMART CheckResult ID should not be empty")
		}
		// On non-Linux: must return SKIP (library unavailable)
		if runtime.GOOS != "linux" && result.Severity != SeveritySkip {
			t.Errorf("expected SKIP on %s, got %s: %s", runtime.GOOS, result.Severity, result.Message)
		}
		// On Linux CI without physical SMART devices: SKIP or INFO expected
		if runtime.GOOS == "linux" && result.Severity == SeverityFail {
			t.Logf("SMART check FAIL (may be expected on CI): id=%s msg=%s", result.ID, result.Message)
		}
	}
}
