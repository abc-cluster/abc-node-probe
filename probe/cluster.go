package probe

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/abc-cluster/abc-node-probe/internal"
)

var clusterCategoryOrder = []string{
	"characteristics",
	"controller_health",
	"cluster",
	"partitions",
	"capacity",
	"queue",
	"pending_reasons",
	"workload",
	"workload_detail",
	"policy",
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
	case "controller_health":
		return []CheckResult{clusterControllerHealthCheck(scheduler)}
	case "cluster":
		return []CheckResult{clusterSchedulerCheck(scheduler), clusterNodeSummaryCheck(scheduler)}
	case "partitions":
		return []CheckResult{
			clusterPartitionInventoryCheck(scheduler),
			clusterPartitionConstraintsCheck(scheduler),
		}
	case "capacity":
		return []CheckResult{clusterCapacitySummaryCheck(scheduler)}
	case "queue":
		return []CheckResult{clusterQueueCheck(scheduler)}
	case "pending_reasons":
		return []CheckResult{clusterPendingReasonsCheck(scheduler)}
	case "workload":
		return []CheckResult{clusterWorkloadCheck(scheduler)}
	case "workload_detail":
		return []CheckResult{clusterWorkloadDetailCheck(scheduler)}
	case "policy":
		return []CheckResult{clusterPolicySummaryCheck(scheduler)}
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

func clusterControllerHealthCheck(scheduler string) CheckResult {
	start := time.Now()
	switch scheduler {
	case "slurm":
		if !commandExists("scontrol") {
			return skippedClusterResult("controller.health.summary", "Controller Health", "scontrol not available; skipping controller health", start)
		}
		out, err := runCommand("scontrol", "ping")
		if err != nil {
			return failClusterResult("controller.health.summary", "Controller Health", "slurmctld health check failed", err, start)
		}
		sev := SeverityInfo
		msg := strings.TrimSpace(out)
		if strings.Contains(strings.ToLower(msg), " is up") {
			sev = SeverityPass
		}
		return CheckResult{
			ID:         "controller.health.summary",
			Category:   "controller_health",
			Name:       "Controller Health",
			Severity:   sev,
			Message:    msg,
			Value:      msg,
			DurationMs: time.Since(start).Milliseconds(),
		}
	case "pbs":
		return skippedClusterResult("controller.health.summary", "Controller Health", "controller health check for PBS not implemented yet", start)
	default:
		return skippedClusterResult("controller.health.summary", "Controller Health", "scheduler unknown; skipping controller health", start)
	}
}

func clusterPartitionInventoryCheck(scheduler string) CheckResult {
	start := time.Now()
	switch scheduler {
	case "slurm":
		out, err := runCommand("sinfo", "-h", "-o", "%P|%a|%l|%D|%f|%G")
		if err != nil {
			return failClusterResult("partitions.inventory.summary", "Partition Inventory", "failed to query Slurm partitions", err, start)
		}
		seen := map[string]struct{}{}
		total := 0
		constrained := 0
		for _, line := range nonEmptyLines(out) {
			parts := strings.Split(line, "|")
			if len(parts) < 6 {
				continue
			}
			name := strings.TrimPrefix(strings.TrimSpace(parts[0]), "*")
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			total++
			features := strings.TrimSpace(parts[4])
			gres := strings.TrimSpace(parts[5])
			if (features != "" && features != "(null)") || (gres != "" && gres != "(null)") {
				constrained++
			}
		}
		return CheckResult{
			ID:         "partitions.inventory.summary",
			Category:   "partitions",
			Name:       "Partition Inventory",
			Severity:   SeverityInfo,
			Message:    fmt.Sprintf("%d partitions detected, %d with explicit feature/GRES constraints", total, constrained),
			Value:      total,
			Unit:       "partitions",
			DurationMs: time.Since(start).Milliseconds(),
		}
	case "pbs":
		return skippedClusterResult("partitions.inventory.summary", "Partition Inventory", "partition inventory for PBS not implemented yet", start)
	default:
		return skippedClusterResult("partitions.inventory.summary", "Partition Inventory", "scheduler unknown; skipping partition inventory", start)
	}
}

func clusterPartitionConstraintsCheck(scheduler string) CheckResult {
	start := time.Now()
	switch scheduler {
	case "slurm":
		out, err := runCommand("scontrol", "show", "partition", "-o")
		if err != nil {
			return failClusterResult("partitions.constraints.summary", "Partition Constraints", "failed to query Slurm partition constraints", err, start)
		}
		lines := nonEmptyLines(out)
		if len(lines) == 0 {
			return CheckResult{
				ID:         "partitions.constraints.summary",
				Category:   "partitions",
				Name:       "Partition Constraints",
				Severity:   SeverityWarn,
				Message:    "no partition constraint data returned",
				DurationMs: time.Since(start).Milliseconds(),
			}
		}

		entries := make([]map[string]string, 0, len(lines))
		for _, line := range lines {
			fields := strings.Fields(line)
			kv := map[string]string{}
			for _, f := range fields {
				parts := strings.SplitN(f, "=", 2)
				if len(parts) != 2 {
					continue
				}
				kv[parts[0]] = parts[1]
			}
			name := strings.TrimPrefix(kv["PartitionName"], "*")
			if name == "" {
				continue
			}
			entry := map[string]string{
				"partition":         name,
				"state":             kvOrDefault(kv, "State", "unknown"),
				"max_time":          kvOrDefault(kv, "MaxTime", "unknown"),
				"max_nodes":         kvOrDefault(kv, "MaxNodes", "unknown"),
				"max_cpus_per_node": kvOrDefault(kv, "MaxCPUsPerNode", "unknown"),
				"max_mem_per_cpu":   kvOrDefault(kv, "MaxMemPerCPU", "unlimited"),
				"default_mem_per_cpu": kvOrDefault(kv, "DefMemPerCPU", "unset"),
				"default":           kvOrDefault(kv, "Default", "NO"),
				"priority_tier":     kvOrDefault(kv, "PriorityTier", "unknown"),
				"oversubscribe":     kvOrDefault(kv, "OverSubscribe", "unknown"),
				"preempt_mode":      kvOrDefault(kv, "PreemptMode", "unknown"),
				"qos":               kvOrDefault(kv, "QoS", "none"),
				"allow_accounts":    kvOrDefault(kv, "AllowAccounts", "all"),
				"deny_accounts":     kvOrDefault(kv, "DenyAccounts", "none"),
				"allow_qos":         kvOrDefault(kv, "AllowQos", "all"),
				"deny_qos":          kvOrDefault(kv, "DenyQos", "none"),
				"allow_groups":      kvOrDefault(kv, "AllowGroups", "all"),
				"deny_groups":       kvOrDefault(kv, "DenyGroups", "none"),
				"nodeset":           kvOrDefault(kv, "Nodes", "unknown"),
			}
			entries = append(entries, entry)
		}

		if len(entries) == 0 {
			return CheckResult{
				ID:         "partitions.constraints.summary",
				Category:   "partitions",
				Name:       "Partition Constraints",
				Severity:   SeverityWarn,
				Message:    "partition constraints could not be parsed",
				DurationMs: time.Since(start).Milliseconds(),
			}
		}

		return CheckResult{
			ID:         "partitions.constraints.summary",
			Category:   "partitions",
			Name:       "Partition Constraints",
			Severity:   SeverityInfo,
			Message:    fmt.Sprintf("%d partition constraint profiles collected", len(entries)),
			Value:      entries,
			DurationMs: time.Since(start).Milliseconds(),
		}
	case "pbs":
		return skippedClusterResult("partitions.constraints.summary", "Partition Constraints", "partition constraints for PBS not implemented yet", start)
	default:
		return skippedClusterResult("partitions.constraints.summary", "Partition Constraints", "scheduler unknown; skipping partition constraints", start)
	}
}

func clusterCapacitySummaryCheck(scheduler string) CheckResult {
	start := time.Now()
	switch scheduler {
	case "slurm":
		out, err := runCommand("sinfo", "-h", "-o", "%C")
		if err != nil {
			return failClusterResult("capacity.cluster.cpu.summary", "Cluster Capacity Summary", "failed to query Slurm capacity", err, start)
		}
		alloc := 0
		idle := 0
		total := 0
		for _, line := range nonEmptyLines(out) {
			parts := strings.Split(strings.TrimSpace(line), "/")
			if len(parts) != 4 {
				continue
			}
			alloc += atoiSafe(parts[0])
			idle += atoiSafe(parts[1])
			total += atoiSafe(parts[3])
		}
		utilPct := 0
		if total > 0 {
			utilPct = int(float64(alloc) / float64(total) * 100.0)
		}
		return CheckResult{
			ID:         "capacity.cluster.cpu.summary",
			Category:   "capacity",
			Name:       "Cluster Capacity Summary",
			Severity:   SeverityInfo,
			Message:    fmt.Sprintf("allocated=%d cpu, idle=%d cpu, total=%d cpu (utilization=%d%%)", alloc, idle, total, utilPct),
			Value:      total,
			Unit:       "cpu",
			DurationMs: time.Since(start).Milliseconds(),
		}
	case "pbs":
		return skippedClusterResult("capacity.cluster.cpu.summary", "Cluster Capacity Summary", "capacity summary for PBS not implemented yet", start)
	default:
		return skippedClusterResult("capacity.cluster.cpu.summary", "Cluster Capacity Summary", "scheduler unknown; skipping capacity summary", start)
	}
}

func clusterPendingReasonsCheck(scheduler string) CheckResult {
	start := time.Now()
	switch scheduler {
	case "slurm":
		out, err := runCommand("squeue", "-h", "-t", "PD", "-o", "%r")
		if err != nil {
			return failClusterResult("pending_reasons.cluster.summary", "Pending Reasons Summary", "failed to query Slurm pending reasons", err, start)
		}
		reasonCount := map[string]int{}
		totalPending := 0
		for _, line := range nonEmptyLines(out) {
			totalPending++
			reason := strings.TrimSpace(line)
			if reason == "" {
				reason = "unknown"
			}
			reasonCount[reason]++
		}
		if totalPending == 0 {
			return CheckResult{
				ID:         "pending_reasons.cluster.summary",
				Category:   "pending_reasons",
				Name:       "Pending Reasons Summary",
				Severity:   SeverityPass,
				Message:    "no pending jobs",
				Value:      0,
				Unit:       "jobs",
				DurationMs: time.Since(start).Milliseconds(),
			}
		}
		type kv struct {
			k string
			v int
		}
		pairs := make([]kv, 0, len(reasonCount))
		for k, v := range reasonCount {
			pairs = append(pairs, kv{k: k, v: v})
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].v > pairs[j].v })
		top := pairs[0]
		return CheckResult{
			ID:         "pending_reasons.cluster.summary",
			Category:   "pending_reasons",
			Name:       "Pending Reasons Summary",
			Severity:   SeverityWarn,
			Message:    fmt.Sprintf("%d pending jobs; top reason=%s (%d)", totalPending, top.k, top.v),
			Value:      totalPending,
			Unit:       "jobs",
			DurationMs: time.Since(start).Milliseconds(),
		}
	case "pbs":
		return skippedClusterResult("pending_reasons.cluster.summary", "Pending Reasons Summary", "pending reason breakdown for PBS not implemented yet", start)
	default:
		return skippedClusterResult("pending_reasons.cluster.summary", "Pending Reasons Summary", "scheduler unknown; skipping pending reasons", start)
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

func clusterWorkloadDetailCheck(scheduler string) CheckResult {
	start := time.Now()
	switch scheduler {
	case "slurm":
		out, err := runCommand("squeue", "-h", "-o", "%u|%a|%T")
		if err != nil {
			return failClusterResult("workload_detail.hpc.accounts_users.summary", "HPC Workload Detail", "failed to query Slurm workload detail", err, start)
		}
		users := map[string]struct{}{}
		accounts := map[string]struct{}{}
		running := 0
		pending := 0
		for _, line := range nonEmptyLines(out) {
			parts := strings.Split(line, "|")
			if len(parts) != 3 {
				continue
			}
			users[strings.TrimSpace(parts[0])] = struct{}{}
			accounts[strings.TrimSpace(parts[1])] = struct{}{}
			state := strings.ToLower(strings.TrimSpace(parts[2]))
			if strings.Contains(state, "running") {
				running++
			}
			if strings.Contains(state, "pending") {
				pending++
			}
		}
		return CheckResult{
			ID:         "workload_detail.hpc.accounts_users.summary",
			Category:   "workload_detail",
			Name:       "HPC Workload Detail",
			Severity:   SeverityInfo,
			Message:    fmt.Sprintf("users=%d accounts=%d running=%d pending=%d", len(users), len(accounts), running, pending),
			DurationMs: time.Since(start).Milliseconds(),
		}
	case "pbs":
		return skippedClusterResult("workload_detail.hpc.accounts_users.summary", "HPC Workload Detail", "workload detail for PBS not implemented yet", start)
	default:
		return skippedClusterResult("workload_detail.hpc.accounts_users.summary", "HPC Workload Detail", "scheduler unknown; skipping workload detail", start)
	}
}

func clusterPolicySummaryCheck(scheduler string) CheckResult {
	start := time.Now()
	switch scheduler {
	case "slurm":
		if !commandExists("sacctmgr") {
			return skippedClusterResult("policy.hpc.qos.summary", "HPC Policy Summary", "sacctmgr not available; skipping policy summary", start)
		}
		out, err := runCommand("sacctmgr", "show", "qos", "format=Name", "-n", "-P")
		if err != nil {
			return failClusterResult("policy.hpc.qos.summary", "HPC Policy Summary", "failed to query Slurm QoS policies", err, start)
		}
		qosCount := 0
		for _, line := range nonEmptyLines(out) {
			if strings.TrimSpace(line) != "" {
				qosCount++
			}
		}
		sev := SeverityInfo
		msg := fmt.Sprintf("%d QoS policies discovered", qosCount)
		if qosCount == 0 {
			sev = SeverityWarn
			msg = "no QoS policies discovered"
		}
		return CheckResult{
			ID:         "policy.hpc.qos.summary",
			Category:   "policy",
			Name:       "HPC Policy Summary",
			Severity:   sev,
			Message:    msg,
			Value:      qosCount,
			Unit:       "qos",
			DurationMs: time.Since(start).Milliseconds(),
		}
	case "pbs":
		return skippedClusterResult("policy.hpc.qos.summary", "HPC Policy Summary", "policy summary for PBS not implemented yet", start)
	default:
		return skippedClusterResult("policy.hpc.qos.summary", "HPC Policy Summary", "scheduler unknown; skipping policy summary", start)
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

func kvOrDefault(m map[string]string, key, fallback string) string {
	v := strings.TrimSpace(m[key])
	if v == "" || v == "(null)" || v == "N/A" {
		return fallback
	}
	return v
}

func (r CheckResult) withDuration(ms int64) CheckResult {
	r.DurationMs = ms
	return r
}
