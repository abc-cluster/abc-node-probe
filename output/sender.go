package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/abc-cluster/abc-node-probe/probe"
)

// WriteFile writes the probe report as formatted JSON to the given file path.
func WriteFile(path string, r *probe.ProbeReport) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// SendReport POSTs the report JSON to the control plane API.
// It retries up to 3 times with exponential backoff (1s, 2s, 4s).
func SendReport(apiEndpoint, token string, r *probe.ProbeReport) error {
	body, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshalling report: %w", err)
	}

	url := apiEndpoint + "/v1/nodes/probe"

	var lastErr error
	backoff := time.Second

	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			time.Sleep(backoff)
			backoff *= 2
		}

		if err := doPost(url, token, body); err != nil {
			lastErr = err
			fmt.Fprintf(os.Stderr, "send attempt %d/3 failed: %v\n", attempt, err)
			continue
		}
		return nil
	}

	return fmt.Errorf("all 3 send attempts failed: %w", lastErr)
}

type apiResponse struct {
	NodeID          string `json:"node_id"`
	AdmissionStatus string `json:"admission_status"`
	Message         string `json:"message"`
}

func doPost(url, token string, body []byte) error {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("POST failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var apiResp apiResponse
		if err := json.Unmarshal(respBody, &apiResp); err == nil {
			fmt.Printf("API response: node_id=%s admission_status=%s message=%s\n",
				apiResp.NodeID, apiResp.AdmissionStatus, apiResp.Message)
		} else {
			fmt.Printf("API response: %s\n", string(respBody))
		}
		return nil
	}

	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
}
