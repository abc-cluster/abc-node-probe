//go:build linux

package probe

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/vishvananda/netlink"
)

func checkNetworkInterfaces() CheckResult {
	start := time.Now()
	id := "network.interfaces.up"
	cat := "network"

	links, err := netlink.LinkList()
	if err != nil {
		return CheckResult{
			ID: id, Category: cat, Name: "Network Interfaces",
			Severity: SeverityWarn, Message: fmt.Sprintf("could not enumerate network interfaces: %v", err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	var upIfaces []string
	for _, link := range links {
		attrs := link.Attrs()
		if attrs.Flags&net.FlagUp != 0 && attrs.Name != "lo" {
			upIfaces = append(upIfaces, fmt.Sprintf("%s(mtu=%d)", attrs.Name, attrs.MTU))
		}
	}

	if len(upIfaces) == 0 {
		return CheckResult{
			ID: id, Category: cat, Name: "Network Interfaces",
			Severity: SeverityFail, Message: "no non-loopback network interfaces are UP",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	return CheckResult{
		ID: id, Category: cat, Name: "Network Interfaces",
		Severity: SeverityPass,
		Message:  fmt.Sprintf("%d interface(s) up: %s", len(upIfaces), strings.Join(upIfaces, ", ")),
		Value:    len(upIfaces),
		DurationMs: time.Since(start).Milliseconds(),
	}
}
