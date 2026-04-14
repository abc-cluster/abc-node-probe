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
		return name == "squeue" || name == "sinfo" || name == "scontrol"
	}
	runCommand = func(name string, args ...string) (string, error) {
		switch {
		case name == "scontrol" && strings.Join(args, " ") == "ping":
			return "Slurmctld(primary) at ctrl is UP", nil
		case name == "scontrol" && strings.Join(args, " ") == "show partition -o":
			return "PartitionName=debug Default=YES MaxTime=30:00 MaxNodes=1 MaxCPUsPerNode=2 State=UP QoS=debug_qos AllowAccounts=ALL AllowQos=ALL AllowGroups=ALL Nodes=node-a\n", nil
		case name == "sinfo" && strings.Join(args, " ") == "-h -o %D|%T":
			return "10|idle\n2|down\n", nil
		case name == "sinfo" && strings.Join(args, " ") == "-h -o %P|%a|%l|%D|%f|%G":
			return "debug*|up|30:00|1|(null)|(null)\ncompute|up|4:00:00|1|gpu|gpu:1\n", nil
		case name == "sinfo" && strings.Join(args, " ") == "-h -o %C":
			return "2/6/0/8\n", nil
		case name == "squeue" && strings.Join(args, " ") == "-h -t PD -o %i":
			return "1001\n1002\n", nil
		case name == "squeue" && strings.Join(args, " ") == "-h -t PD -o %r":
			return "Priority\nResources\nPriority\n", nil
		case name == "squeue" && strings.Join(args, " ") == "-h -o %T":
			return "RUNNING\nPENDING\nRUNNING\n", nil
		case name == "squeue" && strings.Join(args, " ") == "-h -o %u|%a|%T":
			return "alice|research|RUNNING\nbob|research|PENDING\n", nil
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
	if len(report.Results) != 12 {
		t.Fatalf("len(Results) = %d, want 12", len(report.Results))
	}
	if report.Summary.WarnCount == 0 {
		t.Fatal("expected a WARN due to pending queue jobs")
	}
}
