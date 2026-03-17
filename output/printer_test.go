package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/abc-cluster/abc-node-probe/probe"
)

func makeTestReport(failCount, warnCount int) *probe.ProbeReport {
	var results []probe.CheckResult
	for i := 0; i < failCount; i++ {
		results = append(results, probe.CheckResult{
			ID: "test.check.fail", Category: "test",
			Name: "Test Fail", Severity: probe.SeverityFail, Message: "fail",
		})
	}
	for i := 0; i < warnCount; i++ {
		results = append(results, probe.CheckResult{
			ID: "test.check.warn", Category: "test",
			Name: "Test Warn", Severity: probe.SeverityWarn, Message: "warn",
		})
	}

	r := &probe.ProbeReport{
		SchemaVersion: "1.0",
		ProbeVersion:  "test",
		NodeHostname:  "testhost",
		NodeRole:      "compute",
		Jurisdiction:  "ZA",
		Timestamp:     time.Now().UTC(),
		Results:       results,
	}
	// Compute summary via exported computeSummary — access via probe package internals
	// For test purposes, manually set summary
	r.Summary = probe.ProbeSummary{
		TotalChecks: len(results),
		FailCount:   failCount,
		WarnCount:   warnCount,
		Eligible:    failCount == 0,
	}
	return r
}

func TestPrintReport_EligibleWhenNoFails(t *testing.T) {
	var buf bytes.Buffer
	r := makeTestReport(0, 1)
	PrintReport(&buf, r)

	out := buf.String()
	if !strings.Contains(out, "NODE ELIGIBLE TO JOIN: YES") {
		t.Errorf("expected eligible message, got:\n%s", out)
	}
}

func TestPrintReport_NotEligibleWhenFails(t *testing.T) {
	var buf bytes.Buffer
	r := makeTestReport(1, 0)
	PrintReport(&buf, r)

	out := buf.String()
	if !strings.Contains(out, "NODE ELIGIBLE TO JOIN: NO") {
		t.Errorf("expected not eligible message, got:\n%s", out)
	}
}

func TestPrintReport_SummaryCountsPresent(t *testing.T) {
	var buf bytes.Buffer
	r := makeTestReport(2, 3)
	PrintReport(&buf, r)

	out := buf.String()
	if !strings.Contains(out, "SUMMARY:") {
		t.Error("expected SUMMARY line in output")
	}
}
