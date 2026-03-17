// Package internal provides helpers for reading /proc and /sys paths.
package internal

import (
	"os"
	"path/filepath"
	"strings"
)

// ReadProcFile reads the content of a /proc or /sys file and returns it trimmed.
func ReadProcFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// ReadProcFileLines reads a /proc or /sys file and returns its lines, trimmed.
func ReadProcFileLines(path string) ([]string, error) {
	content, err := ReadProcFile(path)
	if err != nil {
		return nil, err
	}
	if content == "" {
		return nil, nil
	}
	return strings.Split(content, "\n"), nil
}

// ProcFileExists reports whether a /proc or /sys file exists.
func ProcFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ReadSysDir reads all entries in a /sys directory and returns their names.
func ReadSysDir(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

// ReadSysFile reads a single sysfs attribute file.
func ReadSysFile(base, attr string) (string, error) {
	return ReadProcFile(filepath.Join(base, attr))
}

// KernelVersion returns the kernel version string from /proc/version.
func KernelVersion() (string, error) {
	return ReadProcFile("/proc/version")
}

// Hostname returns the system hostname from /proc/sys/kernel/hostname.
func Hostname() (string, error) {
	return ReadProcFile("/proc/sys/kernel/hostname")
}

// IsRoot reports whether the current process is running as root (UID 0).
func IsRoot() bool {
	return os.Getuid() == 0
}

// InGroup reports whether the current process belongs to the named group.
// It checks supplementary group IDs against /etc/group by matching GID.
func InGroup(groupName string) bool {
	content, err := os.ReadFile("/etc/group")
	if err != nil {
		return false
	}
	gids, err := os.Getgroups()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(content), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 4 {
			continue
		}
		if parts[0] != groupName {
			continue
		}
		// parts[2] is the GID
		var gid int
		if _, err := parseGID(parts[2], &gid); err != nil {
			continue
		}
		for _, g := range gids {
			if g == gid {
				return true
			}
		}
	}
	return false
}

func parseGID(s string, out *int) (bool, error) {
	var v int
	for _, c := range s {
		if c < '0' || c > '9' {
			return false, nil
		}
		v = v*10 + int(c-'0')
	}
	*out = v
	return true, nil
}
