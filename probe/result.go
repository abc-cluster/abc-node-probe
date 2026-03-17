// Package probe contains check implementations and result types.
package probe

import (
	"time"
)

// Severity represents the outcome level of a single check.
type Severity string

const (
	SeverityPass Severity = "PASS"
	SeverityWarn Severity = "WARN"
	SeverityFail Severity = "FAIL"
	SeveritySkip Severity = "SKIP"
	SeverityInfo Severity = "INFO"
)

// CheckResult holds the outcome of a single probe check.
type CheckResult struct {
	ID         string            `json:"id"`
	Category   string            `json:"category"`
	Name       string            `json:"name"`
	Severity   Severity          `json:"severity"`
	Message    string            `json:"message"`
	Value      interface{}       `json:"value,omitempty"`
	Unit       string            `json:"unit,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	DurationMs int64             `json:"duration_ms"`
}

// ProbeReport is the canonical output of a probe run.
type ProbeReport struct {
	SchemaVersion string        `json:"schema_version"`
	ProbeVersion  string        `json:"probe_version"`
	NodeHostname  string        `json:"node_hostname"`
	NodeRole      string        `json:"node_role"`
	Jurisdiction  string        `json:"jurisdiction"`
	Timestamp     time.Time     `json:"timestamp"`
	DurationMs    int64         `json:"total_duration_ms"`
	Summary       ProbeSummary  `json:"summary"`
	Results       []CheckResult `json:"results"`
}

// ProbeSummary aggregates the check outcome counts.
type ProbeSummary struct {
	TotalChecks int  `json:"total_checks"`
	PassCount   int  `json:"pass_count"`
	WarnCount   int  `json:"warn_count"`
	FailCount   int  `json:"fail_count"`
	SkipCount   int  `json:"skip_count"`
	InfoCount   int  `json:"info_count"`
	Eligible    bool `json:"eligible_to_join"`
}

// computeSummary derives a ProbeSummary from a slice of CheckResults.
func computeSummary(results []CheckResult) ProbeSummary {
	s := ProbeSummary{}
	s.TotalChecks = len(results)
	for _, r := range results {
		switch r.Severity {
		case SeverityPass:
			s.PassCount++
		case SeverityWarn:
			s.WarnCount++
		case SeverityFail:
			s.FailCount++
		case SeveritySkip:
			s.SkipCount++
		case SeverityInfo:
			s.InfoCount++
		}
	}
	s.Eligible = s.FailCount == 0
	return s
}
