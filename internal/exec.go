package internal

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

const defaultSubprocessTimeout = 10 * time.Second

// RunCommand executes a command with a 10-second timeout and returns combined stdout.
// It never returns an error on non-zero exit; callers should check output.
func RunCommand(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultSubprocessTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		return buf.String(), err
	}
	return buf.String(), nil
}

// CommandExists reports whether a binary exists in PATH.
func CommandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
