package output

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abc-cluster/abc-node-probe/probe"
)

func testReport() *probe.ProbeReport {
	return &probe.ProbeReport{
		SchemaVersion: "1.0",
		ProbeVersion:  "test",
		NodeHostname:  "testnode",
		NodeRole:      "compute",
		Jurisdiction:  "ZA",
		Timestamp:     time.Now().UTC(),
	}
}

func TestSendReport_Success(t *testing.T) {
	var receivedToken string
	var receivedBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedToken = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"node_id":          "test-uuid",
			"admission_status": "pending_review",
			"message":          "received",
		})
	}))
	defer srv.Close()

	err := SendReport(srv.URL, "test-token", testReport())
	if err != nil {
		t.Fatalf("SendReport failed: %v", err)
	}

	if receivedToken != "Bearer test-token" {
		t.Errorf("Authorization header = %q, want Bearer test-token", receivedToken)
	}

	if receivedBody["schema_version"] != "1.0" {
		t.Errorf("request body missing schema_version")
	}
}

func TestSendReport_Retry_ThenSuccess(t *testing.T) {
	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"node_id":          "test-uuid",
			"admission_status": "approved",
		})
	}))
	defer srv.Close()

	err := SendReport(srv.URL, "", testReport())
	if err != nil {
		t.Fatalf("SendReport should succeed after retry, got: %v", err)
	}
	if attempt != 2 {
		t.Errorf("expected 2 attempts, got %d", attempt)
	}
}

func TestSendReport_AllRetriesFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	err := SendReport(srv.URL, "", testReport())
	if err == nil {
		t.Error("expected error when all retries fail")
	}
}
