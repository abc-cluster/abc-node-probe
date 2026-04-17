package probe

import (
	"encoding/json"
	"testing"
	"time"
)

func TestProbeReportJSONRoundTrip(t *testing.T) {
	report := &ProbeReport{
		SchemaVersion: "1.0",
		ProbeVersion:  "0.1.0",
		Evaluated:     true,
		NodeHostname:  "test-node",
		NodeRole:      "compute",
		Jurisdiction:  "ZA",
		Timestamp:     time.Now().UTC(),
		DurationMs:    1234,
		Results: []CheckResult{
			{
				ID:         "hardware.cpu.architecture",
				Category:   "hardware",
				Name:       "CPU Architecture",
				Severity:   SeverityPass,
				Message:    "x86_64",
				Value:      "x86_64",
				DurationMs: 5,
			},
			{
				ID:         "compliance.jurisdiction.declared",
				Category:   "compliance",
				Name:       "Jurisdiction Declared",
				Severity:   SeverityFail,
				Message:    "jurisdiction not declared",
				DurationMs: 1,
			},
		},
	}
	report.Summary = computeSummary(report.Results)

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded ProbeReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.SchemaVersion != "1.0" {
		t.Errorf("SchemaVersion = %q, want 1.0", decoded.SchemaVersion)
	}
	if decoded.NodeHostname != "test-node" {
		t.Errorf("NodeHostname = %q, want test-node", decoded.NodeHostname)
	}
	if len(decoded.Results) != 2 {
		t.Errorf("len(Results) = %d, want 2", len(decoded.Results))
	}
	if decoded.Results[0].Severity != SeverityPass {
		t.Errorf("Results[0].Severity = %q, want PASS", decoded.Results[0].Severity)
	}
	if decoded.Results[1].Severity != SeverityFail {
		t.Errorf("Results[1].Severity = %q, want FAIL", decoded.Results[1].Severity)
	}
}

func TestComputeSummary(t *testing.T) {
	results := []CheckResult{
		{Severity: SeverityPass},
		{Severity: SeverityPass},
		{Severity: SeverityWarn},
		{Severity: SeverityFail},
		{Severity: SeveritySkip},
		{Severity: SeverityInfo},
	}

	s := computeSummary(results)

	if s.TotalChecks != 6 {
		t.Errorf("TotalChecks = %d, want 6", s.TotalChecks)
	}
	if s.PassCount != 2 {
		t.Errorf("PassCount = %d, want 2", s.PassCount)
	}
	if s.WarnCount != 1 {
		t.Errorf("WarnCount = %d, want 1", s.WarnCount)
	}
	if s.FailCount != 1 {
		t.Errorf("FailCount = %d, want 1", s.FailCount)
	}
	if s.SkipCount != 1 {
		t.Errorf("SkipCount = %d, want 1", s.SkipCount)
	}
	if s.InfoCount != 1 {
		t.Errorf("InfoCount = %d, want 1", s.InfoCount)
	}
	if s.Eligible {
		t.Error("Eligible should be false when FailCount > 0")
	}
}

func TestComputeSummaryEligible(t *testing.T) {
	results := []CheckResult{
		{Severity: SeverityPass},
		{Severity: SeverityWarn},
		{Severity: SeveritySkip},
	}

	s := computeSummary(results)
	if !s.Eligible {
		t.Error("Eligible should be true when FailCount == 0")
	}
}

func TestSeverityValues(t *testing.T) {
	// Ensure severity constants are valid JSON string values
	for _, sev := range []Severity{SeverityPass, SeverityWarn, SeverityFail, SeveritySkip, SeverityInfo} {
		data, err := json.Marshal(sev)
		if err != nil {
			t.Errorf("json.Marshal(%q) failed: %v", sev, err)
		}
		var decoded Severity
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Errorf("json.Unmarshal failed for %q: %v", sev, err)
		}
		if decoded != sev {
			t.Errorf("round-trip: got %q, want %q", decoded, sev)
		}
	}
}
