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
	SkipCategories []string
	FailFast       bool
	ProbeVersion   string
	Timeout        time.Duration
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

// Run orchestrates all check categories and returns a ProbeReport.
func Run(cfg Config) (*ProbeReport, error) {
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

		results := runCategoryParallel(checks, &failFast, cfg.FailFast)
		sort.Slice(results, func(i, j int) bool {
			return results[i].ID < results[j].ID
		})
		allResults = append(allResults, results...)
	}

	report := &ProbeReport{
		SchemaVersion: "1.0",
		ProbeVersion:  cfg.ProbeVersion,
		NodeHostname:  hostname,
		NodeRole:      cfg.NodeRole,
		Jurisdiction:  cfg.Jurisdiction,
		Timestamp:     time.Now().UTC(),
		DurationMs:    time.Since(start).Milliseconds(),
		Results:       allResults,
		Summary:       computeSummary(allResults),
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
