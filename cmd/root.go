package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/abc-cluster/abc-node-probe/output"
	"github.com/abc-cluster/abc-node-probe/probe"
)

var (
	version   string
	buildTime string
	gitCommit string
)

type flags struct {
	jurisdiction   string
	nodeRole       string
	probeScope     string
	hpcScheduler   string
	apiEndpoint    string
	apiToken       string
	mode           string
	outputFile     string
	skipCategories string
	failFast       bool
	evaluate       bool
	jsonOnly       bool
	nomadMode      bool
	timeout        time.Duration
	showVersion    bool
}

func Execute(ver, bt, gc string) {
	version = ver
	buildTime = bt
	gitCommit = gc

	if err := newRootCmd().Execute(); err != nil {
		os.Exit(3)
	}
}

func newRootCmd() *cobra.Command {
	f := &flags{}

	cmd := &cobra.Command{
		Use:   "abc-node-probe",
		Short: "Assess node or cluster readiness in the ABC-cluster Nomad/Tailscale network",
		Long: `abc-node-probe is a read-only assessment instrument that checks whether a Linux
node meets the requirements to join the ABC-cluster Nomad/Tailscale hybrid compute network.

It never modifies system state.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(f)
		},
	}

	cmd.Flags().StringVar(&f.jurisdiction, "jurisdiction", "", "ISO 3166-1 alpha-2 country code (REQUIRED for compliance checks). Example: --jurisdiction=ZA")
	cmd.Flags().StringVar(&f.nodeRole, "node-role", "compute", "One of: compute, storage, scheduler, gateway")
	cmd.Flags().StringVar(&f.probeScope, "probe-scope", "node", "Probe scope: node | cluster")
	cmd.Flags().StringVar(&f.hpcScheduler, "hpc-scheduler", "auto", "HPC scheduler for cluster scope: auto | slurm | pbs")
	cmd.Flags().StringVar(&f.apiEndpoint, "api-endpoint", "", "ABC-cluster control plane API base URL (required for --mode=send)")
	cmd.Flags().StringVar(&f.apiToken, "api-token", "", "Bearer token for API auth (or set ABC_PROBE_TOKEN env var)")
	cmd.Flags().StringVar(&f.mode, "mode", "stdout", "Output mode: stdout | file | send")
	cmd.Flags().StringVar(&f.outputFile, "output-file", "", "Path to write JSON report (required for --mode=file)")
	cmd.Flags().StringVar(&f.skipCategories, "skip-categories", "", "Comma-separated check categories to skip. Node scope: hardware,storage,smart,network,os,compliance,security. Cluster scope: characteristics,controller_health,cluster,partitions,capacity,queue,pending_reasons,workload,workload_detail,policy")
	cmd.Flags().BoolVar(&f.failFast, "fail-fast", false, "Stop after first FAIL result (only with --evaluate)")
	cmd.Flags().BoolVar(&f.evaluate, "evaluate", false, "Compute PASS/WARN/FAIL/INFO severities, summary, and exit codes for admission-style reporting; without it, checks still run but only observations (values/messages) are returned")
	cmd.Flags().BoolVar(&f.jsonOnly, "json", false, "Print raw JSON to stdout (suppresses coloured output)")
	cmd.Flags().BoolVar(&f.nomadMode, "nomad-mode", false, "Nomad-compatible mode: always exit 0 (success) since probe task completed; use JSON output to check node readiness")
	cmd.Flags().DurationVar(&f.timeout, "timeout", 120*time.Second, "Overall probe timeout")
	cmd.Flags().BoolVar(&f.showVersion, "version", false, "Print version and exit")

	return cmd
}

func run(f *flags) error {
	if f.showVersion {
		fmt.Printf("abc-node-probe %s (built %s, commit %s)\n", version, buildTime, gitCommit)
		return nil
	}

	// Resolve env var overrides
	if f.apiToken == "" {
		f.apiToken = os.Getenv("ABC_PROBE_TOKEN")
	}
	if f.apiEndpoint == "" {
		f.apiEndpoint = os.Getenv("ABC_PROBE_API")
	}
	if f.jurisdiction == "" {
		f.jurisdiction = os.Getenv("ABC_PROBE_JURISDICTION")
	}
	if f.hpcScheduler == "auto" {
		if envScheduler := os.Getenv("ABC_PROBE_HPC_SCHEDULER"); envScheduler != "" {
			f.hpcScheduler = envScheduler
		}
	}

	// Validate mode
	switch f.mode {
	case "stdout", "file", "send":
	default:
		return fmt.Errorf("invalid --mode %q: must be stdout, file, or send", f.mode)
	}

	if f.mode == "file" && f.outputFile == "" {
		return fmt.Errorf("--output-file is required when --mode=file")
	}
	if f.mode == "send" && f.apiEndpoint == "" {
		return fmt.Errorf("--api-endpoint is required when --mode=send")
	}

	switch f.probeScope {
	case "node", "cluster":
	default:
		return fmt.Errorf("invalid --probe-scope %q: must be node or cluster", f.probeScope)
	}
	switch f.hpcScheduler {
	case "auto", "slurm", "pbs":
	default:
		return fmt.Errorf("invalid --hpc-scheduler %q: must be auto, slurm, or pbs", f.hpcScheduler)
	}

	// Validate node role
	if f.probeScope == "node" {
		switch f.nodeRole {
		case "compute", "storage", "scheduler", "gateway":
		default:
			return fmt.Errorf("invalid --node-role %q: must be compute, storage, scheduler, or gateway", f.nodeRole)
		}
	}

	// Parse skip categories
	var skipCats []string
	if f.skipCategories != "" {
		skipCats = strings.Split(f.skipCategories, ",")
		for i, c := range skipCats {
			skipCats[i] = strings.TrimSpace(c)
		}
	}

	cfg := probe.Config{
		Jurisdiction:   f.jurisdiction,
		NodeRole:       f.nodeRole,
		ProbeScope:     f.probeScope,
		HPCScheduler:   f.hpcScheduler,
		SkipCategories: skipCats,
		FailFast:       f.failFast,
		Evaluate:       f.evaluate,
		ProbeVersion:   version,
		Timeout:        f.timeout,
	}

	report, err := probe.Run(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "probe error: %v\n", err)
		return err
	}

	exitCode := exitCodeForReport(report)
	// In nomad-mode, always exit 0 since the probe task completed successfully.
	// Node readiness is conveyed via JSON output/API, not exit code.
	if f.nomadMode {
		exitCode = 0
	}

	switch f.mode {
	case "stdout":
		if f.jsonOnly {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(report)
		} else {
			output.PrintReport(os.Stdout, report)
			fmt.Fprintln(os.Stdout)
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(report)
		}

	case "file":
		if err := output.WriteFile(f.outputFile, report); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write report: %v\n", err)
			os.Exit(3)
		}
		hostname := report.NodeHostname
		s := report.Summary
		fmt.Printf("Report written to %s\n", f.outputFile)
		if report.Evaluated {
			eligible := "ELIGIBLE"
			if !s.Eligible {
				eligible = "NOT ELIGIBLE"
			}
			fmt.Printf("Summary (%s): %d checks — %d PASS, %d WARN, %d FAIL — %s\n",
				hostname, s.TotalChecks, s.PassCount, s.WarnCount, s.FailCount, eligible)
		} else {
			fmt.Printf("Summary (%s): %d checks — observations only (no severity scoring; re-run with --evaluate for admission summary)\n",
				hostname, s.TotalChecks)
		}

	case "send":
		output.PrintReport(os.Stdout, report)
		fmt.Fprintln(os.Stdout)

		token := f.apiToken
		if err := output.SendReport(f.apiEndpoint, token, report); err != nil {
			fmt.Fprintf(os.Stderr, "failed to send report: %v\n", err)
			os.Exit(2)
		}
	}

	os.Exit(exitCode)
	return nil
}

func exitCodeForReport(r *probe.ProbeReport) int {
	if !r.Evaluated {
		return 0
	}
	if r.Summary.FailCount > 0 {
		return 2
	}
	if r.Summary.WarnCount > 0 {
		return 1
	}
	return 0
}
