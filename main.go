package main

import (
	"github.com/abc-cluster/abc-node-probe/cmd"
)

// Version, BuildTime, and GitCommit are injected at build time via ldflags.
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	cmd.Execute(Version, BuildTime, GitCommit)
}
