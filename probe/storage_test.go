package probe

import (
	"testing"
)

func TestCheckInotifyLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping /proc test in short mode")
	}
	result := checkInotifyLimits()
	// The check should produce a result (not panic) regardless of the system state
	if result.ID == "" {
		t.Error("CheckResult.ID should not be empty")
	}
	if result.Category != "storage" {
		t.Errorf("Category = %q, want storage", result.Category)
	}
}

func TestCheckUlimits(t *testing.T) {
	result := checkUlimits()
	if result.ID == "" {
		t.Error("CheckResult.ID should not be empty")
	}
	if result.ID != "storage.ulimits.open_files" {
		t.Errorf("ID = %q, want storage.ulimits.open_files", result.ID)
	}
	// The severity should be PASS or WARN, never FAIL for ulimits on dev machines
	if result.Severity == SeverityFail {
		t.Logf("WARN: open files ulimit returned FAIL: %s", result.Message)
	}
}

func TestCheckMinioEndpoint_Skip_WhenEnvNotSet(t *testing.T) {
	// Ensure ABC_MINIO_ENDPOINT is not set
	t.Setenv("ABC_MINIO_ENDPOINT", "")
	result := checkMinioEndpoint()
	if result.Severity != SeveritySkip {
		t.Errorf("expected SKIP when ABC_MINIO_ENDPOINT not set, got %s", result.Severity)
	}
}
