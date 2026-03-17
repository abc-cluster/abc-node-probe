//go:build integration && linux

package probe

import (
	"testing"
)

func TestSmartChecks_GracefulSkipWithNoPhysicalDevice(t *testing.T) {
	cfg := Config{NodeRole: "compute"}
	checks := smartChecks(cfg)

	if len(checks) == 0 {
		t.Fatal("smartChecks returned no check functions")
	}

	for _, fn := range checks {
		result := fn()
		// On a CI runner without physical SMART-capable devices, we expect SKIP or INFO
		if result.Severity == SeverityFail {
			t.Errorf("SMART check returned FAIL on CI (expected SKIP): id=%s msg=%s", result.ID, result.Message)
		}
		if result.ID == "" {
			t.Error("SMART CheckResult ID should not be empty")
		}
	}
}

func TestSmartChecks_DeviceMetadataPopulated(t *testing.T) {
	cfg := Config{NodeRole: "compute"}
	checks := smartChecks(cfg)

	for _, fn := range checks {
		result := fn()
		if result.Severity == SeverityPass || result.Severity == SeverityWarn || result.Severity == SeverityFail {
			if result.Metadata == nil {
				t.Errorf("SMART result %s has no metadata", result.ID)
			}
			if result.Metadata["device"] == "" {
				t.Errorf("SMART result %s missing device in metadata", result.ID)
			}
		}
	}
}
