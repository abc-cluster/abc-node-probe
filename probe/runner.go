package probe

import (
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Config holds all runtime configuration for a probe run.
type Config struct {
	Jurisdiction   string
	NodeRole       string
	ProbeScope     string
	HPCScheduler   string
	SkipCategories []string
	FailFast       bool
	// Evaluate enables PASS/WARN/FAIL/INFO scoring. When false, checks still run
	// and return values and messages, but severities other than SKIP are cleared.
	Evaluate     bool
	ProbeVersion string
	Timeout      time.Duration
}

// categoryOrder defines the deterministic execution order of check categories.
var categoryOrder = []string{
	"hardware",
	"storage",
	"smart",
	"network",
	"os",
	"compliance",
	"security",
}

// observationsOnlyResults strips severity for checks that produced a scored outcome,
// preserving SKIP so callers can still see checks that did not run.
func observationsOnlyResults(results []CheckResult) []CheckResult {
	out := make([]CheckResult, len(results))
	for i, r := range results {
		if r.Severity != SeveritySkip {
			r.Severity = ""
		}
		out[i] = r
	}
	return out
}

// Run orchestrates all check categories and returns a ProbeReport.
func Run(cfg Config) (*ProbeReport, error) {
	if cfg.ProbeScope == "cluster" {
		report, err := runCluster(cfg)
		if err != nil {
			return nil, err
		}
		if !cfg.Evaluate {
			report.Results = observationsOnlyResults(report.Results)
			report.Summary = computeSummary(report.Results)
		}
		report.Evaluated = cfg.Evaluate
		return report, nil
	}

	start := time.Now()

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	skipSet := make(map[string]bool, len(cfg.SkipCategories))
	for _, c := range cfg.SkipCategories {
		skipSet[c] = true
	}

	var failFast atomic.Bool

	var allResults []CheckResult

	for _, cat := range categoryOrder {
		if skipSet[cat] {
			continue
		}
		if failFast.Load() {
			break
		}

		checks := checksForCategory(cat, cfg)
		if len(checks) == 0 {
			continue
		}

		results := runCategoryParallel(checks, &failFast, cfg.FailFast && cfg.Evaluate)
		sort.Slice(results, func(i, j int) bool {
			return results[i].ID < results[j].ID
		})
		allResults = append(allResults, results...)
	}

	results := allResults
	if !cfg.Evaluate {
		results = observationsOnlyResults(results)
	}

	report := &ProbeReport{
		SchemaVersion: "1.0",
		ProbeVersion:  cfg.ProbeVersion,
		Evaluated:     cfg.Evaluate,
		ProbeScope:    "node",
		NodeHostname:  hostname,
		NodeRole:      cfg.NodeRole,
		Jurisdiction:  cfg.Jurisdiction,
		Timestamp:     time.Now().UTC(),
		DurationMs:    time.Since(start).Milliseconds(),
		Results:       results,
		Summary:       computeSummary(results),
	}

	return report, nil
}

// checkFunc is a function that performs a single check and returns a result.
type checkFunc func() CheckResult

// runCategoryParallel runs all checks for a category concurrently.
func runCategoryParallel(checks []checkFunc, failFast *atomic.Bool, stopOnFail bool) []CheckResult {
	results := make([]CheckResult, len(checks))
	var wg sync.WaitGroup
	wg.Add(len(checks))

	for i, fn := range checks {
		i, fn := i, fn
		go func() {
			defer wg.Done()
			if failFast.Load() {
				return
			}
			r := fn()
			results[i] = r
			if stopOnFail && r.Severity == SeverityFail {
				failFast.Store(true)
			}
		}()
	}

	wg.Wait()

	// Remove zero-value results (skipped due to fail-fast)
	var out []CheckResult
	for _, r := range results {
		if r.ID != "" {
			out = append(out, r)
		}
	}
	return out
}

// checksForCategory returns the list of check functions for a given category.
func checksForCategory(cat string, cfg Config) []checkFunc {
	switch cat {
	case "hardware":
		return hardwareChecks(cfg)
	case "storage":
		return storageChecks(cfg)
	case "smart":
		return smartChecks(cfg)
	case "network":
		return networkChecks(cfg)
	case "os":
		return osChecks(cfg)
	case "compliance":
		return complianceChecks(cfg)
	case "security":
		return securityChecks(cfg)
	}
	return nil
}
