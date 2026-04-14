package probe

import (
	"testing"
)

// TestCheckNetworkSpeedtest_ReturnsValidResult ensures the speedtest check returns a valid result.
// This test is marked as requiring internet connectivity.
func TestCheckNetworkSpeedtest_ReturnsValidResult(t *testing.T) {
	// Note: This test requires internet connectivity and may take 30-60 seconds.
	// Run with: go test -run TestCheckNetworkSpeedtest_ReturnsValidResult
	
	result := checkNetworkSpeedtest()
	
	if result.ID == "" {
		t.Error("expected non-empty ID")
	}
	
	if result.ID != "network.speedtest.throughput" {
		t.Errorf("expected ID 'network.speedtest.throughput', got '%s'", result.ID)
	}
	
	if result.Category != "network" {
		t.Errorf("expected category 'network', got '%s'", result.Category)
	}
	
	if result.Name != "Network Speedtest" {
		t.Errorf("expected name 'Network Speedtest', got '%s'", result.Name)
	}
	
	// Check that severity is either INFO or WARN
	if result.Severity != SeverityInfo && result.Severity != SeverityWarn {
		t.Errorf("expected severity to be INFO or WARN, got '%s'", result.Severity)
	}
	
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
	
	// Check that duration is recorded
	if result.DurationMs <= 0 {
		t.Errorf("expected positive duration_ms, got %d", result.DurationMs)
	}
	
	// Verify the Value field contains expected speed metrics
	if result.Value == nil {
		t.Error("expected non-nil Value field")
	} else {
		valueMap, ok := result.Value.(map[string]interface{})
		if !ok {
			t.Errorf("expected Value to be map[string]interface{}, got %T", result.Value)
		}
		
		// Check for required metrics
		if _, hasDownload := valueMap["download_mbps"]; !hasDownload {
			t.Error("expected 'download_mbps' in Value")
		}
		if _, hasUpload := valueMap["upload_mbps"]; !hasUpload {
			t.Error("expected 'upload_mbps' in Value")
		}
		if _, hasLatency := valueMap["latency_ms"]; !hasLatency {
			t.Error("expected 'latency_ms' in Value")
		}
		if _, hasJitter := valueMap["jitter_ms"]; !hasJitter {
			t.Error("expected 'jitter_ms' in Value")
		}
	}
	
	// Verify important metadata fields exist
	if result.Metadata == nil {
		t.Error("expected non-nil Metadata field")
	} else {
		requiredMetadata := []string{
			"server_name",
			"server_location",
			"download_mbps",
			"upload_mbps",
			"latency_ms",
			"client_ip",
			"client_isp",
		}
		
		for _, key := range requiredMetadata {
			if _, ok := result.Metadata[key]; !ok {
				t.Errorf("expected '%s' in Metadata", key)
			}
		}
	}
}

// TestCheckNetworkSpeedtest_MeasuresReasonableValues ensures the speedtest result contains sensible values.
// This is a sanity check that download/upload speeds are positive and within expected ranges.
func TestCheckNetworkSpeedtest_MeasuresReasonableValues(t *testing.T) {
	// Note: This test requires internet connectivity and may take 30-60 seconds.
	
	result := checkNetworkSpeedtest()
	
	// Only run assertions if the test completed successfully
	if result.Severity == SeverityWarn && (
		result.Message == "could not fetch speedtest servers" ||
			result.Message == "could not find speedtest servers" ||
			result.Message == "could not fetch user info") {
		t.Skip("speedtest test skipped due to connectivity issues")
	}
	
	if result.Value == nil {
		return
	}
	
	valueMap, ok := result.Value.(map[string]interface{})
	if !ok {
		return
	}
	
	// Verify speeds are positive numbers
	if dlDL, ok := valueMap["download_mbps"].(float64); ok {
		if dlDL < 0 {
			t.Errorf("expected positive download_mbps, got %.2f", dlDL)
		}
	}
	
	if ul, ok := valueMap["upload_mbps"].(float64); ok {
		if ul < 0 {
			t.Errorf("expected positive upload_mbps, got %.2f", ul)
		}
	}
	
	// Verify latency is a reasonable value (positive and typically < 500ms)
	if lat, ok := valueMap["latency_ms"].(int64); ok {
		if lat <= 0 {
			t.Errorf("expected positive latency_ms, got %d", lat)
		}
		if lat > 1000 {
			t.Logf("warning: latency is rather high: %d ms (expected < 1000 ms)", lat)
		}
	}
}
