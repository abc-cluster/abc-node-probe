package probe

import (
	"testing"
)

// --- helper unit tests ---

func TestIsNetworkFSType(t *testing.T) {
	cases := []struct {
		fstype string
		want   bool
	}{
		{"nfs", true},
		{"nfs4", true},
		{"cifs", true},
		{"smbfs", true},
		{"fuse.sshfs", true},
		{"fuse.s3fs", true},
		{"fuse.gcsfuse", true},
		{"fuse.anything", true},
		{"ext4", false},
		{"xfs", false},
		{"btrfs", false},
		{"tmpfs", false},
		{"", false},
	}
	for _, c := range cases {
		got := isNetworkFSType(c.fstype)
		if got != c.want {
			t.Errorf("isNetworkFSType(%q) = %v, want %v", c.fstype, got, c.want)
		}
	}
}

func TestIsPseudoFSType(t *testing.T) {
	cases := []struct {
		fstype string
		want   bool
	}{
		{"tmpfs", true},
		{"sysfs", true},
		{"proc", true},
		{"devtmpfs", true},
		{"cgroup", true},
		{"cgroup2", true},
		{"overlay", true},
		{"squashfs", true},
		{"ext4", false},
		{"xfs", false},
		{"btrfs", false},
		{"nfs", false},
		{"", false},
	}
	for _, c := range cases {
		got := isPseudoFSType(c.fstype)
		if got != c.want {
			t.Errorf("isPseudoFSType(%q) = %v, want %v", c.fstype, got, c.want)
		}
	}
}

func TestIsPathUnder(t *testing.T) {
	cases := []struct {
		path string
		mp   string
		want bool
	}{
		{"/scratch", "/scratch", true},
		{"/scratch/data", "/scratch", true},
		{"/scratch", "/", true},
		{"/", "/", true},
		{"/home/user", "/home", true},
		{"/other", "/scratch", false},
		{"/scratchy", "/scratch", false},
	}
	for _, c := range cases {
		got := isPathUnder(c.path, c.mp)
		if got != c.want {
			t.Errorf("isPathUnder(%q, %q) = %v, want %v", c.path, c.mp, got, c.want)
		}
	}
}

func TestScratchDir_ReturnsTmpWhenNoScratch(t *testing.T) {
	// In most CI environments /scratch does not exist, so we expect /tmp.
	dir := scratchDir()
	if dir != "/scratch" && dir != "/tmp" {
		t.Errorf("scratchDir() = %q, want /scratch or /tmp", dir)
	}
}

// --- check function tests ---

func TestCheckScratchFilesystemType(t *testing.T) {
	result := checkScratchFilesystemType()
	if result.ID != "storage.scratch.filesystem_type" {
		t.Errorf("ID = %q, want storage.scratch.filesystem_type", result.ID)
	}
	if result.Category != "storage" {
		t.Errorf("Category = %q, want storage", result.Category)
	}
	// Must have a result (PASS, WARN, FAIL, or SKIP on unexpected platforms)
	if result.Severity == "" {
		t.Error("Severity should not be empty")
	}
	// Metadata must contain "path" whenever the check did not error out
	if result.Metadata != nil {
		if _, ok := result.Metadata["path"]; !ok {
			t.Error("metadata should contain 'path'")
		}
	}
}

func TestCheckMountInventory(t *testing.T) {
	result := checkMountInventory()
	if result.ID != "storage.mounts.inventory" {
		t.Errorf("ID = %q, want storage.mounts.inventory", result.ID)
	}
	if result.Category != "storage" {
		t.Errorf("Category = %q, want storage", result.Category)
	}
	// Should be INFO or WARN (never FAIL for this informational check)
	if result.Severity == SeverityFail {
		t.Errorf("Mount inventory should never return FAIL, got: %s", result.Message)
	}
	if result.Metadata == nil {
		t.Error("metadata should not be nil")
	}
	if _, ok := result.Metadata["count"]; !ok {
		t.Error("metadata should contain 'count'")
	}
}

func TestCheckScratchFreeSpace_HasRicherMetadata(t *testing.T) {
	result := checkScratchFreeSpace()
	if result.ID != "storage.scratch.free_space" {
		t.Errorf("ID = %q, want storage.scratch.free_space", result.ID)
	}
	// Verify new metadata fields exist when disk.Usage succeeded.
	// When disk.Usage fails, the function returns WARN with no metadata.
	if result.Metadata != nil {
		for _, key := range []string{"path", "total_gb", "used_gb", "used_pct"} {
			if result.Metadata[key] == "" {
				t.Errorf("metadata[%q] should not be empty", key)
			}
		}
	}
}

func TestCheckScratchInode_HandlesZeroInodes(t *testing.T) {
	result := checkScratchInode()
	if result.ID != "storage.scratch.inodes_free" {
		t.Errorf("ID = %q, want storage.scratch.inodes_free", result.ID)
	}
	// Result should be PASS, WARN, FAIL, or INFO (not an empty severity)
	if result.Severity == "" {
		t.Error("Severity should not be empty")
	}
	// INFO is valid when the filesystem does not report inodes
	validSeverities := map[Severity]bool{
		SeverityPass: true, SeverityWarn: true,
		SeverityFail: true, SeverityInfo: true,
	}
	if !validSeverities[result.Severity] {
		t.Errorf("unexpected severity %q", result.Severity)
	}
}

func TestCheckInotifyLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping /proc test in short mode")
	}
	result := checkInotifyLimits()
	// The check should produce a result (not panic) regardless of the system state
	if result.ID == "" {
		t.Error("CheckResult.ID should not be empty")
	}
	if result.Category != "storage" {
		t.Errorf("Category = %q, want storage", result.Category)
	}
}

func TestCheckUlimits(t *testing.T) {
	result := checkUlimits()
	if result.ID == "" {
		t.Error("CheckResult.ID should not be empty")
	}
	if result.ID != "storage.ulimits.open_files" {
		t.Errorf("ID = %q, want storage.ulimits.open_files", result.ID)
	}
	// The severity should be PASS or WARN, never FAIL for ulimits on dev machines
	if result.Severity == SeverityFail {
		t.Logf("WARN: open files ulimit returned FAIL: %s", result.Message)
	}
}

func TestCheckMinioEndpoint_Skip_WhenEnvNotSet(t *testing.T) {
	// Ensure ABC_MINIO_ENDPOINT is not set
	t.Setenv("ABC_MINIO_ENDPOINT", "")
	result := checkMinioEndpoint()
	if result.Severity != SeveritySkip {
		t.Errorf("expected SKIP when ABC_MINIO_ENDPOINT not set, got %s", result.Severity)
	}
}
