package probe

import "testing"

func TestObservationsOnlyResults(t *testing.T) {
	in := []CheckResult{
		{ID: "a", Severity: SeverityPass},
		{ID: "b", Severity: SeverityFail},
		{ID: "c", Severity: SeveritySkip},
		{ID: "d", Severity: SeverityInfo},
	}
	out := observationsOnlyResults(in)

	if out[0].Severity != "" || out[1].Severity != "" {
		t.Errorf("expected cleared severity for pass/fail, got %q %q", out[0].Severity, out[1].Severity)
	}
	if out[2].Severity != SeveritySkip {
		t.Errorf("expected SKIP preserved, got %q", out[2].Severity)
	}
	if out[3].Severity != "" {
		t.Errorf("expected cleared INFO severity, got %q", out[3].Severity)
	}
}

func TestComputeSummaryIgnoresEmptySeverity(t *testing.T) {
	s := computeSummary([]CheckResult{
		{Severity: ""},
		{Severity: SeveritySkip},
	})
	if s.TotalChecks != 2 {
		t.Fatalf("TotalChecks = %d", s.TotalChecks)
	}
	if s.SkipCount != 1 || s.PassCount != 0 || s.FailCount != 0 {
		t.Fatalf("counts: pass=%d fail=%d skip=%d", s.PassCount, s.FailCount, s.SkipCount)
	}
	if !s.Eligible {
		t.Error("Eligible should be true when no FAIL severity present")
	}
}
