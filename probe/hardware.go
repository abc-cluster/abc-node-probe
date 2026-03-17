package probe

import (
	"fmt"
	"time"

	"github.com/jaypipes/ghw"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"

	"github.com/abc-cluster/abc-node-probe/internal"
)

func hardwareChecks(cfg Config) []checkFunc {
	return []checkFunc{
		checkCPUArchitecture,
		checkCPUCores,
		checkMemoryTotal,
		func() CheckResult { return checkNUMATopology(cfg) },
		func() CheckResult { return checkGPU(cfg) },
	}
}

func checkCPUArchitecture() CheckResult {
	start := time.Now()
	id := "hardware.cpu.architecture"
	cat := "hardware"

	arch, err := internal.ReadProcFile("/proc/sys/kernel/arch")
	if err != nil {
		// Fall back to uname approach
		out, err2 := internal.RunCommand("uname", "-m")
		if err2 != nil {
			return CheckResult{
				ID: id, Category: cat, Name: "CPU Architecture",
				Severity: SeverityWarn, Message: "could not determine CPU architecture",
				DurationMs: time.Since(start).Milliseconds(),
			}
		}
		arch = out
	}

	return CheckResult{
		ID: id, Category: cat, Name: "CPU Architecture",
		Severity: SeverityPass, Value: arch,
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkCPUCores() CheckResult {
	start := time.Now()
	id := "hardware.cpu.logical_cores"
	cat := "hardware"

	counts, err := cpu.Counts(true)
	if err != nil || counts == 0 {
		return CheckResult{
			ID: id, Category: cat, Name: "CPU Logical Cores",
			Severity: SeverityWarn, Message: "could not read — kernel may not support this metric",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	sev := SeverityPass
	msg := ""
	if counts < 2 {
		sev = SeverityWarn
		msg = "fewer than 2 logical CPU cores detected"
	}

	return CheckResult{
		ID: id, Category: cat, Name: "CPU Logical Cores",
		Severity: sev, Message: msg, Value: counts, Unit: "cores",
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkMemoryTotal() CheckResult {
	start := time.Now()
	id := "hardware.memory.total_ram"
	cat := "hardware"

	vmStat, err := mem.VirtualMemory()
	if err != nil || vmStat.Total == 0 {
		return CheckResult{
			ID: id, Category: cat, Name: "Total RAM",
			Severity: SeverityWarn, Message: "could not read — kernel may not support this metric",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	gb := float64(vmStat.Total) / (1024 * 1024 * 1024)

	sev := SeverityInfo
	msg := ""
	if gb < 4 {
		sev = SeverityWarn
		msg = fmt.Sprintf("only %.1f GB RAM — minimum recommended is 4 GB", gb)
	}

	return CheckResult{
		ID: id, Category: cat, Name: "Total RAM",
		Severity: sev, Message: msg, Value: fmt.Sprintf("%.1f", gb), Unit: "GB",
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkNUMATopology(cfg Config) CheckResult {
	start := time.Now()
	id := "hardware.numa.topology"
	cat := "hardware"

	if !internal.IsRoot() {
		return CheckResult{
			ID: id, Category: cat, Name: "NUMA Topology",
			Severity: SeveritySkip, Message: "requires root — skipping",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	topo, err := ghw.Topology()
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "NUMA Topology",
			Severity: SeverityWarn, Message: fmt.Sprintf("could not read NUMA topology: %v", err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	nodeCount := len(topo.Nodes)
	return CheckResult{
		ID: id, Category: cat, Name: "NUMA Topology",
		Severity: SeverityInfo, Value: nodeCount, Unit: "nodes",
		Message: fmt.Sprintf("%d NUMA node(s) detected", nodeCount),
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkGPU(cfg Config) CheckResult {
	start := time.Now()
	id := "hardware.gpu.devices"
	cat := "hardware"

	if !internal.IsRoot() {
		return CheckResult{
			ID: id, Category: cat, Name: "GPU Devices",
			Severity: SeveritySkip, Message: "requires root — skipping",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	gpuInfo, err := ghw.GPU()
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "GPU Devices",
			Severity: SeverityInfo, Message: fmt.Sprintf("GPU enumeration not available: %v", err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	count := len(gpuInfo.GraphicsCards)
	return CheckResult{
		ID: id, Category: cat, Name: "GPU Devices",
		Severity: SeverityInfo, Value: count, Unit: "devices",
		Message: fmt.Sprintf("%d GPU device(s) detected", count),
		DurationMs: time.Since(start).Milliseconds(),
	}
}
