//go:build integration

package probe

import (
	"runtime"
	"testing"
)

func TestCheckNamespaces_PassOnModernLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("kernel namespace check only meaningful on Linux (running on %s)", runtime.GOOS)
	}
	result := checkNamespaces()
	if result.Severity == SeverityFail {
		t.Errorf("kernel namespaces check FAIL on modern Linux: %s", result.Message)
	}
}

func TestCheckCgroupsVersion_ReturnsResult(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("cgroups check only applicable on Linux (running on %s)", runtime.GOOS)
	}
	result := checkCgroupsVersion()
	if result.ID == "" {
		t.Error("expected non-empty result ID")
	}
	if result.Severity == SeverityFail {
		t.Errorf("cgroups check unexpectedly FAIL: %s", result.Message)
	}
}

func TestOSChecks_ProduceResults(t *testing.T) {
	cfg := Config{NodeRole: "compute"}
	checks := osChecks(cfg)
	if len(checks) == 0 {
		t.Fatal("osChecks returned no checks")
	}
	for _, fn := range checks {
		result := fn()
		if result.ID == "" {
			t.Errorf("os check returned empty ID (platform: %s)", runtime.GOOS)
		}
	}
}
