// Package output handles rendering and delivery of probe reports.
package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"

	"github.com/abc-cluster/abc-node-probe/probe"
)

var (
	passColor = color.New(color.FgGreen, color.Bold)
	warnColor = color.New(color.FgYellow, color.Bold)
	failColor = color.New(color.FgRed, color.Bold)
	skipColor = color.New(color.FgWhite)
	infoColor = color.New(color.FgCyan)
)

const separator = "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

// PrintReport writes a coloured human-readable table for the given report.
func PrintReport(w io.Writer, r *probe.ProbeReport) {
	fmt.Fprintf(w, "abc-node-probe %s — node: %s — role: %s — jurisdiction: %s\n",
		r.ProbeVersion, r.NodeHostname, r.NodeRole, r.Jurisdiction)
	fmt.Fprintln(w, separator)
	fmt.Fprintf(w, "%-14s %-30s %-10s %-14s %s\n", "CATEGORY", "CHECK", "SEVERITY", "VALUE", "MESSAGE")
	fmt.Fprintln(w, separator)

	for _, res := range r.Results {
		// Shorten category for display
		cat := res.Category
		checkName := res.ID
		// Remove category prefix from ID for display
		if strings.HasPrefix(checkName, cat+".") {
			checkName = checkName[len(cat)+1:]
		}

		val := ""
		if res.Value != nil {
			val = fmt.Sprintf("%v", res.Value)
			if res.Unit != "" {
				val += " " + res.Unit
			}
		}

		sevStr := colorSeverity(res.Severity)
		msg := res.Message

		// Truncate long values for table display
		if len(val) > 13 {
			val = val[:10] + "..."
		}
		if len(msg) > 50 {
			msg = msg[:47] + "..."
		}

		fmt.Fprintf(w, "%-14s %-30s %-20s %-14s %s\n", cat, checkName, sevStr, val, msg)
	}

	fmt.Fprintln(w, separator)
	s := r.Summary
	fmt.Fprintf(w, "SUMMARY: %d checks — %d PASS, %d WARN, %d FAIL, %d SKIP\n",
		s.TotalChecks, s.PassCount, s.WarnCount, s.FailCount, s.SkipCount)

	if s.Eligible {
		passColor.Fprintf(w, "NODE ELIGIBLE TO JOIN: YES\n")
	} else {
		failColor.Fprintf(w, "NODE ELIGIBLE TO JOIN: NO (resolve FAIL checks first)\n")
	}
	fmt.Fprintln(w, separator)
}

func colorSeverity(sev probe.Severity) string {
	switch sev {
	case probe.SeverityPass:
		return passColor.Sprint(string(sev))
	case probe.SeverityWarn:
		return warnColor.Sprint(string(sev))
	case probe.SeverityFail:
		return failColor.Sprint(string(sev))
	case probe.SeveritySkip:
		return skipColor.Sprint(string(sev))
	case probe.SeverityInfo:
		return infoColor.Sprint(string(sev))
	}
	return string(sev)
}
