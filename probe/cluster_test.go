package probe

import (
	"fmt"
	"strings"
	"testing"
)

func TestDetectHPCSchedulerAuto(t *testing.T) {
	origExists := commandExists
	t.Cleanup(func() { commandExists = origExists })

	commandExists = func(name string) bool { return name == "squeue" }
	if got := detectHPCScheduler("auto"); got != "slurm" {
		t.Fatalf("detectHPCScheduler(auto) = %q, want slurm", got)
	}
}

func TestRunClusterSlurmQueueAndWorkload(t *testing.T) {
	origExists := commandExists
	origRun := runCommand
	t.Cleanup(func() {
		commandExists = origExists
		runCommand = origRun
	})

	commandExists = func(name string) bool {
		return name == "squeue" || name == "sinfo"
	}
	runCommand = func(name string, args ...string) (string, error) {
		switch {
		case name == "sinfo":
			return "10|idle\n2|down\n", nil
		case name == "squeue" && strings.Join(args, " ") == "-h -t PD -o %i":
			return "1001\n1002\n", nil
		case name == "squeue" && strings.Join(args, " ") == "-h -o %T":
			return "RUNNING\nPENDING\nRUNNING\n", nil
		default:
			return "", fmt.Errorf("unexpected command %s %v", name, args)
		}
	}

	report, err := Run(Config{
		ProbeScope:   "cluster",
		HPCScheduler: "auto",
		ProbeVersion: "test",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if report.ProbeScope != "cluster" {
		t.Fatalf("ProbeScope = %q, want cluster", report.ProbeScope)
	}
	if len(report.Results) != 5 {
		t.Fatalf("len(Results) = %d, want 5", len(report.Results))
	}
	if report.Summary.WarnCount == 0 {
		t.Fatal("expected a WARN due to pending queue jobs")
	}
}
