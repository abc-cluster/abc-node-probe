package probe

import (
	"fmt"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

// networkSpeedtestResult holds detailed speedtest metrics.
type networkSpeedtestResult struct {
	ServerName      string
	ServerLocation  string
	Latency         string
	Jitter          string
	DownloadSpeed   string // Human-readable format (e.g., "123.45 Mbps")
	DownloadRaw     float64 // Raw bytes per second
	UploadSpeed     string
	UploadRaw       float64 // Raw bytes per second
	IssuerCountry   string
	Country         string
	ClientIP        string
	ClientISP       string
}

// checkNetworkSpeedtest performs a network speed test using the speedtest.net API.
// This check is informational and will not cause failures, but will provide valuable
// data about the network performance for cluster node readiness assessment.
//
// Note: This check requires internet connectivity to a speedtest.net server and may
// take 30-60 seconds to complete depending on your network speed.
func checkNetworkSpeedtest() CheckResult {
	start := time.Now()
	id := "network.speedtest.throughput"
	cat := "network"

	// Initialize the speedtest client
	var speedtestClient = speedtest.New()

	// Fetch user information (includes public IP and ISP)
	user, err := speedtestClient.FetchUserInfo()
	if err != nil {
		return CheckResult{
			ID:         id,
			Category:   cat,
			Name:       "Network Speedtest",
			Severity:   SeverityWarn,
			Message:    fmt.Sprintf("could not fetch user info: %v", err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	// Fetch the list of available servers
	serverList, err := speedtestClient.FetchServers()
	if err != nil {
		return CheckResult{
			ID:         id,
			Category:   cat,
			Name:       "Network Speedtest",
			Severity:   SeverityWarn,
			Message:    fmt.Sprintf("could not fetch speedtest servers: %v", err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	// Find the closest server(s)
	targets, err := serverList.FindServer([]int{})
	if err != nil {
		return CheckResult{
			ID:         id,
			Category:   cat,
			Name:       "Network Speedtest",
			Severity:   SeverityWarn,
			Message:    fmt.Sprintf("could not find speedtest servers: %v", err),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	if len(targets) == 0 {
		return CheckResult{
			ID:         id,
			Category:   cat,
			Name:       "Network Speedtest",
			Severity:   SeverityWarn,
			Message:    "no available speedtest servers found",
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	// Use the closest server
	server := targets[0]

	// Perform ping test to get latency
	err = server.PingTest(nil)
	if err != nil {
		return CheckResult{
			ID:       id,
			Category: cat,
			Name:     "Network Speedtest",
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("ping test failed: %v", err),
			Metadata: map[string]string{
				"server_name": server.Name,
				"server_host": server.Host,
				"client_ip":   user.IP,
				"client_isp":  user.Isp,
			},
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	// Perform download test
	err = server.DownloadTest()
	if err != nil {
		return CheckResult{
			ID:       id,
			Category: cat,
			Name:     "Network Speedtest",
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("download test failed: %v", err),
			Metadata: map[string]string{
				"server_name":   server.Name,
				"latency_ms":    fmt.Sprintf("%d", server.Latency.Milliseconds()),
				"client_ip":     user.IP,
			},
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	// Perform upload test
	err = server.UploadTest()
	if err != nil {
		return CheckResult{
			ID:       id,
			Category: cat,
			Name:     "Network Speedtest",
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("upload test failed: %v", err),
			Metadata: map[string]string{
				"server_name":   server.Name,
				"download_mbps": fmt.Sprintf("%.2f", server.DLSpeed.Mbps()),
				"latency_ms":    fmt.Sprintf("%d", server.Latency.Milliseconds()),
				"client_ip":     user.IP,
			},
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	// Build the result message with speedtest data
	dlMbps := server.DLSpeed.Mbps()
	ulMbps := server.ULSpeed.Mbps()
	latencyMs := server.Latency.Milliseconds()
	jitterMs := server.Jitter.Milliseconds()

	message := fmt.Sprintf(
		"Download: %.2f Mbps, Upload: %.2f Mbps, Latency: %d ms, Jitter: %d ms (%s, %s)",
		dlMbps, ulMbps, latencyMs, jitterMs,
		server.Name, server.Country,
	)

	metadata := map[string]string{
		"server_name":      server.Name,
		"server_sponsor":   server.Sponsor,
		"server_location":  fmt.Sprintf("%s, %s", server.Name, server.Country),
		"download_mbps":    fmt.Sprintf("%.2f", dlMbps),
		"upload_mbps":      fmt.Sprintf("%.2f", ulMbps),
		"latency_ms":       fmt.Sprintf("%d", latencyMs),
		"jitter_ms":        fmt.Sprintf("%d", jitterMs),
		"min_latency_ms":   fmt.Sprintf("%d", server.MinLatency.Milliseconds()),
		"max_latency_ms":   fmt.Sprintf("%d", server.MaxLatency.Milliseconds()),
		"client_ip":        user.IP,
		"client_isp":       user.Isp,
		"server_host":      server.Host,
		"server_distance":  fmt.Sprintf("%.2f km", server.Distance),
	}

	// Determine severity based on download speed (optional thresholds)
	// These can be tuned based on what speeds are expected for your cluster
	// Default: WARN if less than 10 Mbps (poor connectivity), otherwise INFO
	severity := SeverityInfo
	if dlMbps < 10 {
		severity = SeverityWarn
	}

	return CheckResult{
		ID:       id,
		Category: cat,
		Name:     "Network Speedtest",
		Severity: severity,
		Message:  message,
		Value: map[string]interface{}{
			"download_mbps": dlMbps,
			"upload_mbps":   ulMbps,
			"latency_ms":    latencyMs,
			"jitter_ms":     jitterMs,
		},
		Metadata:   metadata,
		DurationMs: time.Since(start).Milliseconds(),
	}
}
