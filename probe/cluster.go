package probe

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/abc-cluster/abc-node-probe/internal"
)

var clusterCategoryOrder = []string{
	"characteristics",
	"cluster",
	"queue",
	"workload",
}

var runCommand = internal.RunCommand
var commandExists = internal.CommandExists

func runCluster(cfg Config) (*ProbeReport, error) {
	start := time.Now()
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	scheduler := detectHPCScheduler(cfg.HPCScheduler)
	skipSet := make(map[string]bool, len(cfg.SkipCategories))
	for _, cat := range cfg.SkipCategories {
		skipSet[strings.TrimSpace(cat)] = true
	}

	var allResults []CheckResult
	for _, cat := range clusterCategoryOrder {
		if skipSet[cat] {
			continue
		}
		allResults = append(allResults, checksForClusterCategory(cat, scheduler)...)
	}

	report := &ProbeReport{
		SchemaVersion: "1.0",
		ProbeVersion:  cfg.ProbeVersion,
		ProbeScope:    "cluster",
		NodeHostname:  hostname,
		NodeRole:      "cluster",
		Jurisdiction:  cfg.Jurisdiction,
		Timestamp:     time.Now().UTC(),
		DurationMs:    time.Since(start).Milliseconds(),
		Results:       allResults,
		Summary:       computeSummary(allResults),
	}
	return report, nil
}

func detectHPCScheduler(requested string) string {
	normalized := strings.ToLower(strings.TrimSpace(requested))
	switch normalized {
	case "slurm", "pbs":
		return normalized
	}
	if commandExists("squeue") || commandExists("sinfo") {
		return "slurm"
	}
	if commandExists("qstat") {
		return "pbs"
	}
	return "unknown"
}

func checksForClusterCategory(cat, scheduler string) []CheckResult {
	switch cat {
	case "characteristics":
		return []CheckResult{clusterCharacteristicsCheck(scheduler)}
	case "cluster":
		return []CheckResult{clusterSchedulerCheck(scheduler), clusterNodeSummaryCheck(scheduler)}
	case "queue":
		return []CheckResult{clusterQueueCheck(scheduler)}
	case "workload":
		return []CheckResult{clusterWorkloadCheck(scheduler)}
	default:
		return nil
	}
}

func clusterCharacteristicsCheck(scheduler string) CheckResult {
	start := time.Now()
	tools := []string{}
	if commandExists("sinfo") || commandExists("squeue") {
		tools = append(tools, "slurm-cli")
	}
	if commandExists("qstat") {
		tools = append(tools, "pbs-cli")
	}

	if len(tools) == 0 {
		return CheckResult{
			ID:         "characteristics.cluster.tooling.summary",
			Category:   "characteristics",
			Name:       "Cluster Tooling Characteristics",
			Severity:   SeveritySkip,
			Message:    "no scheduler CLI tools detected on this host",
			Value:      "none",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	msg := fmt.Sprintf("scheduler=%s; tooling=%s", scheduler, strings.Join(tools, ","))
	return CheckResult{
		ID:         "characteristics.cluster.tooling.summary",
		Category:   "characteristics",
		Name:       "Cluster Tooling Characteristics",
		Severity:   SeverityInfo,
		Message:    msg,
		Value:      msg,
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func clusterSchedulerCheck(scheduler string) CheckResult {
	r := CheckResult{
		ID:       "cluster.hpc.scheduler.detected",
		Category: "cluster",
		Name:     "HPC Scheduler Detection",
		Value:    scheduler,
	}
	if scheduler == "unknown" {
		r.Severity = SeveritySkip
		r.Message = "could not detect Slurm/PBS tools on this host"
		return r.withDuration(0)
	}
	r.Severity = SeverityPass
	r.Message = fmt.Sprintf("detected %s scheduler tooling", strings.ToUpper(scheduler))
	return r.withDuration(0)
}

func clusterNodeSummaryCheck(scheduler string) CheckResult {
	start := time.Now()
	switch scheduler {
	case "slurm":
		out, err := runCommand("sinfo", "-h", "-o", "%D|%T")
		if err != nil {
			return failClusterResult("cluster.hpc.nodes.summary", "HPC Node Summary", "failed to query Slurm node state", err, start)
		}
		total := 0
		down := 0
		for _, line := range nonEmptyLines(out) {
			parts := strings.Split(line, "|")
			if len(parts) != 2 {
				continue
			}
			n := atoiSafe(parts[0])
			total += n
			state := strings.ToLower(parts[1])
			if strings.Contains(state, "down") || strings.Contains(state, "drain") {
				down += n
			}
		}
		sev := SeverityInfo
		msg := fmt.Sprintf("%d nodes in Slurm view, %d unavailable", total, down)
		if total == 0 {
			sev = SeverityWarn
			msg = "Slurm reports zero nodes"
		}
		return CheckResult{
			ID:         "cluster.hpc.nodes.summary",
			Category:   "cluster",
			Name:       "HPC Node Summary",
			Severity:   sev,
			Message:    msg,
			Value:      total,
			Unit:       "nodes",
			DurationMs: time.Since(start).Milliseconds(),
		}
	case "pbs":
		out, err := runCommand("qstat", "-Bf")
		if err != nil {
			return failClusterResult("cluster.hpc.nodes.summary", "HPC Node Summary", "failed to query PBS server summary", err, start)
		}
		queued, running := parsePBSQueuedRunning(out)
		return CheckResult{
			ID:         "cluster.hpc.nodes.summary",
			Category:   "cluster",
			Name:       "HPC Node Summary",
			Severity:   SeverityInfo,
			Message:    fmt.Sprintf("PBS view: %d queued jobs, %d running jobs", queued, running),
			DurationMs: time.Since(start).Milliseconds(),
		}
	default:
		return skippedClusterResult("cluster.hpc.nodes.summary", "HPC Node Summary", "scheduler unknown; skipping node summary", start)
	}
}

func clusterQueueCheck(scheduler string) CheckResult {
	start := time.Now()
	switch scheduler {
	case "slurm":
		out, err := runCommand("squeue", "-h", "-t", "PD", "-o", "%i")
		if err != nil {
			return failClusterResult("queue.hpc.pending.summary", "HPC Queue Backlog", "failed to query Slurm pending queue", err, start)
		}
		pending := len(nonEmptyLines(out))
		sev := SeverityPass
		msg := "no pending jobs in scheduler queue"
		if pending > 0 {
			sev = SeverityWarn
			msg = fmt.Sprintf("%d jobs waiting in scheduler queue", pending)
		}
		return CheckResult{
			ID:         "queue.hpc.pending.summary",
			Category:   "queue",
			Name:       "HPC Queue Backlog",
			Severity:   sev,
			Message:    msg,
			Value:      pending,
			Unit:       "jobs",
			DurationMs: time.Since(start).Milliseconds(),
		}
	case "pbs":
		out, err := runCommand("qstat")
		if err != nil {
			return failClusterResult("queue.hpc.pending.summary", "HPC Queue Backlog", "failed to query PBS queue", err, start)
		}
		queued, _ := parsePBSQueuedRunning(out)
		sev := SeverityPass
		msg := "no queued jobs in scheduler queue"
		if queued > 0 {
			sev = SeverityWarn
			msg = fmt.Sprintf("%d jobs queued in PBS", queued)
		}
		return CheckResult{
			ID:         "queue.hpc.pending.summary",
			Category:   "queue",
			Name:       "HPC Queue Backlog",
			Severity:   sev,
			Message:    msg,
			Value:      queued,
			Unit:       "jobs",
			DurationMs: time.Since(start).Milliseconds(),
		}
	default:
		return skippedClusterResult("queue.hpc.pending.summary", "HPC Queue Backlog", "scheduler unknown; skipping queue intel", start)
	}
}

func clusterWorkloadCheck(scheduler string) CheckResult {
	start := time.Now()
	switch scheduler {
	case "slurm":
		out, err := runCommand("squeue", "-h", "-o", "%T")
		if err != nil {
			return failClusterResult("workload.hpc.active.summary", "HPC Workload Summary", "failed to query Slurm workload", err, start)
		}
		total := 0
		running := 0
		pending := 0
		for _, line := range nonEmptyLines(out) {
			total++
			state := strings.ToLower(line)
			if strings.Contains(state, "running") {
				running++
			}
			if strings.Contains(state, "pending") {
				pending++
			}
		}
		return CheckResult{
			ID:         "workload.hpc.active.summary",
			Category:   "workload",
			Name:       "HPC Workload Summary",
			Severity:   SeverityInfo,
			Message:    fmt.Sprintf("%d total jobs, %d running, %d pending", total, running, pending),
			Value:      total,
			Unit:       "jobs",
			DurationMs: time.Since(start).Milliseconds(),
		}
	case "pbs":
		out, err := runCommand("qstat")
		if err != nil {
			return failClusterResult("workload.hpc.active.summary", "HPC Workload Summary", "failed to query PBS workload", err, start)
		}
		queued, running := parsePBSQueuedRunning(out)
		total := queued + running
		return CheckResult{
			ID:         "workload.hpc.active.summary",
			Category:   "workload",
			Name:       "HPC Workload Summary",
			Severity:   SeverityInfo,
			Message:    fmt.Sprintf("%d total jobs, %d running, %d queued", total, running, queued),
			Value:      total,
			Unit:       "jobs",
			DurationMs: time.Since(start).Milliseconds(),
		}
	default:
		return skippedClusterResult("workload.hpc.active.summary", "HPC Workload Summary", "scheduler unknown; skipping workload intel", start)
	}
}

func failClusterResult(id, name, prefix string, err error, start time.Time) CheckResult {
	return CheckResult{
		ID:         id,
		Category:   strings.Split(id, ".")[0],
		Name:       name,
		Severity:   SeverityFail,
		Message:    fmt.Sprintf("%s: %v", prefix, err),
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func skippedClusterResult(id, name, msg string, start time.Time) CheckResult {
	return CheckResult{
		ID:         id,
		Category:   strings.Split(id, ".")[0],
		Name:       name,
		Severity:   SeveritySkip,
		Message:    msg,
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func nonEmptyLines(s string) []string {
	raw := strings.Split(strings.TrimSpace(s), "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func parsePBSQueuedRunning(out string) (queued, running int) {
	for _, line := range nonEmptyLines(out) {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		state := strings.ToUpper(fields[len(fields)-2])
		switch state {
		case "Q":
			queued++
		case "R":
			running++
		}
	}
	return queued, running
}

func atoiSafe(s string) int {
	s = strings.TrimSpace(s)
	val := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return val
		}
		val = (val * 10) + int(ch-'0')
	}
	return val
}

func (r CheckResult) withDuration(ms int64) CheckResult {
	r.DurationMs = ms
	return r
}
