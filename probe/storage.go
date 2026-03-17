package probe

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
)

func storageChecks(cfg Config) []checkFunc {
	return []checkFunc{
		checkScratchFreeSpace,
		checkScratchInode,
		checkScratchIOPS,
		checkMinioEndpoint,
		checkInotifyLimits,
		checkUlimits,
	}
}

func checkScratchFreeSpace() CheckResult {
	start := time.Now()
	id := "storage.scratch.free_space"
	cat := "storage"

	// Use /tmp as scratch if no dedicated scratch mount exists
	scratchPath := "/scratch"
	if _, err := os.Stat(scratchPath); err != nil {
		scratchPath = "/tmp"
	}

	usage, err := disk.Usage(scratchPath)
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "Scratch Free Space",
			Severity: SeverityWarn, Message: fmt.Sprintf("could not read disk usage for %s: %v", scratchPath, err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	freeGB := float64(usage.Free) / (1024 * 1024 * 1024)
	sev := SeverityPass
	msg := ""

	switch {
	case freeGB < 50:
		sev = SeverityFail
		msg = fmt.Sprintf("only %.1f GB free on %s — minimum 50 GB required", freeGB, scratchPath)
	case freeGB < 100:
		sev = SeverityWarn
		msg = fmt.Sprintf("%.1f GB free on %s — recommend at least 100 GB", freeGB, scratchPath)
	}

	return CheckResult{
		ID: id, Category: cat, Name: "Scratch Free Space",
		Severity: sev, Message: msg,
		Value: fmt.Sprintf("%.1f", freeGB), Unit: "GB",
		Metadata:   map[string]string{"path": scratchPath},
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkScratchInode() CheckResult {
	start := time.Now()
	id := "storage.scratch.inodes_free"
	cat := "storage"

	scratchPath := "/scratch"
	if _, err := os.Stat(scratchPath); err != nil {
		scratchPath = "/tmp"
	}

	usage, err := disk.Usage(scratchPath)
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "Scratch Inodes Free",
			Severity: SeverityWarn, Message: fmt.Sprintf("could not read inode info: %v", err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	inodesFreePct := 100.0
	if usage.InodesTotal > 0 {
		inodesFreePct = float64(usage.InodesFree) / float64(usage.InodesTotal) * 100
	}

	sev := SeverityPass
	msg := ""
	if inodesFreePct < 5 {
		sev = SeverityFail
		msg = fmt.Sprintf("only %.1f%% inodes free on %s", inodesFreePct, scratchPath)
	} else if inodesFreePct < 20 {
		sev = SeverityWarn
		msg = fmt.Sprintf("%.1f%% inodes free on %s — running low", inodesFreePct, scratchPath)
	}

	return CheckResult{
		ID: id, Category: cat, Name: "Scratch Inodes Free",
		Severity: sev, Message: msg,
		Value: fmt.Sprintf("%.1f", inodesFreePct), Unit: "%",
		Metadata:   map[string]string{"path": scratchPath},
		DurationMs: time.Since(start).Milliseconds(),
	}
}

// checkScratchIOPS performs a sequential write benchmark using direct I/O writes + fsync.
// It does NOT shell out to fio or dd.
func checkScratchIOPS() CheckResult {
	start := time.Now()
	id := "storage.scratch.write_throughput"
	cat := "storage"

	scratchPath := "/scratch"
	if _, err := os.Stat(scratchPath); err != nil {
		scratchPath = "/tmp"
	}

	tmpFile := filepath.Join(scratchPath, fmt.Sprintf(".nomad-probe-bench-%d", time.Now().UnixNano()))
	f, err := os.Create(tmpFile)
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "Scratch Write Throughput",
			Severity: SeverityWarn, Message: fmt.Sprintf("could not create temp file in %s: %v", scratchPath, err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}
	defer os.Remove(tmpFile)
	defer f.Close()

	const totalBytes = 512 * 1024 * 1024 // 512 MB
	const chunkSize = 4 * 1024 * 1024    // 4 MB

	buf := make([]byte, chunkSize)
	writeStart := time.Now()

	written := 0
	for written < totalBytes {
		n, err := f.Write(buf)
		if err != nil {
			return CheckResult{
				ID: id, Category: cat, Name: "Scratch Write Throughput",
				Severity: SeverityWarn, Message: fmt.Sprintf("write error during benchmark: %v", err),
				DurationMs: time.Since(start).Milliseconds(),
			}
		}
		written += n
		// fsync after each chunk
		if err := f.Sync(); err != nil {
			// non-fatal, continue
		}
	}

	elapsed := time.Since(writeStart)
	mbps := float64(totalBytes) / (1024 * 1024) / elapsed.Seconds()

	sev := SeverityPass
	msg := ""
	if mbps < 100 {
		sev = SeverityWarn
		msg = fmt.Sprintf("write throughput %.1f MB/s is below recommended 100 MB/s", mbps)
	}

	return CheckResult{
		ID: id, Category: cat, Name: "Scratch Write Throughput",
		Severity: sev, Message: msg,
		Value: fmt.Sprintf("%.1f", mbps), Unit: "MB/s",
		Metadata:   map[string]string{"path": scratchPath, "benchmark_size_mb": "512"},
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkMinioEndpoint() CheckResult {
	start := time.Now()
	id := "storage.minio.endpoint_reachable"
	cat := "storage"

	// Default MinIO endpoint — operators can override via ABC_MINIO_ENDPOINT env
	endpoint := os.Getenv("ABC_MINIO_ENDPOINT")
	if endpoint == "" {
		return CheckResult{
			ID: id, Category: cat, Name: "MinIO Endpoint Reachable",
			Severity: SeveritySkip, Message: "ABC_MINIO_ENDPOINT not set — skipping MinIO check",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	conn, err := net.DialTimeout("tcp", endpoint, 5*time.Second)
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "MinIO Endpoint Reachable",
			Severity: SeverityFail, Message: fmt.Sprintf("No route to host: %s", endpoint),
			Metadata:   map[string]string{"endpoint": endpoint},
			DurationMs: time.Since(start).Milliseconds(),
		}
	}
	conn.Close()

	return CheckResult{
		ID: id, Category: cat, Name: "MinIO Endpoint Reachable",
		Severity: SeverityPass, Message: fmt.Sprintf("endpoint %s is reachable", endpoint),
		Metadata:   map[string]string{"endpoint": endpoint},
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkInotifyLimits() CheckResult {
	start := time.Now()
	id := "storage.inotify.max_user_watches"
	cat := "storage"

	const path = "/proc/sys/fs/inotify/max_user_watches"
	var val int
	content, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "Inotify Max User Watches",
			Severity: SeverityWarn, Message: "could not read inotify limit",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}
	fmt.Sscanf(string(content), "%d", &val)

	sev := SeverityPass
	msg := ""
	if val == 0 {
		sev = SeverityWarn
		msg = "could not read — kernel may not support this metric"
	} else if val < 65536 {
		sev = SeverityWarn
		msg = fmt.Sprintf("inotify max_user_watches=%d is low; recommend >= 65536", val)
	}

	return CheckResult{
		ID: id, Category: cat, Name: "Inotify Max User Watches",
		Severity: sev, Message: msg, Value: val,
		DurationMs: time.Since(start).Milliseconds(),
	}
}

