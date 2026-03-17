package probe

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
)

func storageChecks(cfg Config) []checkFunc {
	return []checkFunc{
		checkScratchFreeSpace,
		checkScratchInode,
		checkScratchFilesystemType,
		checkScratchIOPS,
		checkMountInventory,
		checkMinioEndpoint,
		checkInotifyLimits,
		checkUlimits,
	}
}

// scratchDir returns "/scratch" if the directory exists, otherwise "/tmp".
func scratchDir() string {
	if _, err := os.Stat("/scratch"); err == nil {
		return "/scratch"
	}
	return "/tmp"
}

// isPathUnder reports whether path is equal to mp or nested under it.
func isPathUnder(path, mp string) bool {
	if path == mp {
		return true
	}
	if mp == "/" {
		return strings.HasPrefix(path, "/")
	}
	return strings.HasPrefix(path, mp+"/")
}

// fsTypeForPath returns the filesystem type and backing device for the mount
// point that contains path, using a longest-prefix match across all mounts.
// It calls disk.Partitions(true) so that pseudo-filesystems such as tmpfs are
// included and correctly identified.
func fsTypeForPath(path string) (fstype, device string, err error) {
	partitions, err := disk.Partitions(true)
	if err != nil {
		return "", "", fmt.Errorf("could not list partitions: %w", err)
	}
	bestLen := -1
	for _, p := range partitions {
		if !isPathUnder(path, p.Mountpoint) {
			continue
		}
		if len(p.Mountpoint) > bestLen {
			bestLen = len(p.Mountpoint)
			fstype = p.Fstype
			device = p.Device
		}
	}
	if bestLen == -1 {
		return "", "", fmt.Errorf("no mount point found for %s", path)
	}
	return fstype, device, nil
}

// networkFSTypes lists filesystem types that are network-backed.
var networkFSTypes = map[string]bool{
	"nfs": true, "nfs3": true, "nfs4": true,
	"cifs": true, "smbfs": true,
	"fuse.sshfs": true, "fuse.s3fs": true,
	"fuse.glusterfs": true, "fuse.gcsfuse": true, "fuse.cephfs": true,
	"afs": true, "9p": true,
}

// isNetworkFSType reports whether fstype is a network-backed filesystem.
func isNetworkFSType(fstype string) bool {
	lower := strings.ToLower(fstype)
	return networkFSTypes[lower] || strings.HasPrefix(lower, "fuse.")
}

// pseudoFSTypes lists filesystem types that are virtual/pseudo (not real storage).
var pseudoFSTypes = map[string]bool{
	"tmpfs": true, "ramfs": true, "sysfs": true, "proc": true,
	"devtmpfs": true, "devpts": true, "cgroup": true, "cgroup2": true,
	"pstore": true, "securityfs": true, "hugetlbfs": true, "mqueue": true,
	"debugfs": true, "tracefs": true, "fusectl": true, "configfs": true,
	"binfmt_misc": true, "overlay": true, "aufs": true,
	"squashfs": true, "iso9660": true, "udf": true, "autofs": true,
}

// isPseudoFSType reports whether fstype is a virtual or pseudo filesystem.
func isPseudoFSType(fstype string) bool {
	return pseudoFSTypes[strings.ToLower(fstype)]
}

func checkScratchFreeSpace() CheckResult {
	start := time.Now()
	id := "storage.scratch.free_space"
	cat := "storage"

	scratchPath := scratchDir()
	usage, err := disk.Usage(scratchPath)
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "Scratch Free Space",
			Severity: SeverityWarn, Message: fmt.Sprintf("could not read disk usage for %s: %v", scratchPath, err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	freeGB := float64(usage.Free) / (1024 * 1024 * 1024)
	totalGB := float64(usage.Total) / (1024 * 1024 * 1024)
	usedGB := float64(usage.Used) / (1024 * 1024 * 1024)
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
		Metadata: map[string]string{
			"path":     scratchPath,
			"total_gb": fmt.Sprintf("%.1f", totalGB),
			"used_gb":  fmt.Sprintf("%.1f", usedGB),
			"used_pct": fmt.Sprintf("%.1f", usage.UsedPercent),
		},
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func checkScratchInode() CheckResult {
	start := time.Now()
	id := "storage.scratch.inodes_free"
	cat := "storage"

	scratchPath := scratchDir()
	usage, err := disk.Usage(scratchPath)
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "Scratch Inodes Free",
			Severity: SeverityWarn, Message: fmt.Sprintf("could not read inode info: %v", err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	// Some filesystems (e.g. FAT32, certain network mounts) report zero inodes.
	if usage.InodesTotal == 0 {
		return CheckResult{
			ID: id, Category: cat, Name: "Scratch Inodes Free",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("filesystem at %s does not report inode counts", scratchPath),
			Metadata: map[string]string{"path": scratchPath},
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	inodesFreePct := float64(usage.InodesFree) / float64(usage.InodesTotal) * 100

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
		Metadata: map[string]string{
			"path":        scratchPath,
			"inode_total": fmt.Sprintf("%d", usage.InodesTotal),
			"inode_used":  fmt.Sprintf("%d", usage.InodesUsed),
		},
		DurationMs: time.Since(start).Milliseconds(),
	}
}

// checkScratchFilesystemType detects the filesystem type of the scratch path
// and warns when it is memory-backed (tmpfs/ramfs) or network-backed (nfs/cifs/fuse).
// Inspired by duf's per-mount fstype detection via /proc/self/mountinfo.
func checkScratchFilesystemType() CheckResult {
	start := time.Now()
	id := "storage.scratch.filesystem_type"
	cat := "storage"

	path := scratchDir()
	fstype, device, err := fsTypeForPath(path)
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "Scratch Filesystem Type",
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("could not detect filesystem type for %s: %v", path, err),
			Metadata: map[string]string{"path": path},
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	sev := SeverityPass
	msg := fmt.Sprintf("%s is %s (%s)", path, fstype, device)
	switch {
	case fstype == "tmpfs" || fstype == "ramfs":
		sev = SeverityFail
		msg = fmt.Sprintf("scratch path %s is backed by %s — memory-backed storage is not persistent and is unsuitable for compute workloads", path, fstype)
	case isNetworkFSType(fstype):
		sev = SeverityWarn
		msg = fmt.Sprintf("scratch path %s uses a network filesystem (%s) — expect higher latency and reduced throughput compared to local storage", path, fstype)
	}

	return CheckResult{
		ID: id, Category: cat, Name: "Scratch Filesystem Type",
		Severity: sev, Message: msg,
		Value: fstype,
		Metadata: map[string]string{
			"path":   path,
			"fstype": fstype,
			"device": device,
		},
		DurationMs: time.Since(start).Milliseconds(),
	}
}

// checkMountInventory enumerates all local, non-pseudo mount points and reports
// them as an informational result. This mirrors duf's approach of enumerating
// all mounts with their filesystem type and device.
func checkMountInventory() CheckResult {
	start := time.Now()
	id := "storage.mounts.inventory"
	cat := "storage"

	partitions, err := disk.Partitions(false)
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "Mount Point Inventory",
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("could not enumerate mount points: %v", err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	var local []disk.PartitionStat
	for _, p := range partitions {
		if !isPseudoFSType(p.Fstype) {
			local = append(local, p)
		}
	}

	metadata := make(map[string]string, len(local)+1)
	metadata["count"] = fmt.Sprintf("%d", len(local))
	for i, p := range local {
		metadata[fmt.Sprintf("mount_%d", i)] = fmt.Sprintf("%s type=%s device=%s", p.Mountpoint, p.Fstype, p.Device)
	}

	return CheckResult{
		ID: id, Category: cat, Name: "Mount Point Inventory",
		Severity: SeverityInfo,
		Message:  fmt.Sprintf("%d local mount point(s) detected", len(local)),
		Value:    len(local),
		Metadata: metadata,
		DurationMs: time.Since(start).Milliseconds(),
	}
}

// checkScratchIOPS performs a sequential write benchmark using direct I/O writes + fsync.
// It does NOT shell out to fio or dd.
func checkScratchIOPS() CheckResult {
	start := time.Now()
	id := "storage.scratch.write_throughput"
	cat := "storage"

	scratchPath := scratchDir()
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

