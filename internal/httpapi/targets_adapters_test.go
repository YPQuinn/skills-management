package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestAdaptersRESTPreInit pins that the adapter table with on-demand
// detection is available before initialization (the setup page uses it)
// and returns exactly the eight built-in adapters with snake_case statuses.
func TestAdaptersRESTPreInit(t *testing.T) {
	ts := startServer(t) // no setup
	resp := ts.do(t, "GET", "/api/v1/targets/adapters", "", "", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("adapters: got %d", resp.StatusCode)
	}
	var envelope struct {
		Items []struct {
			Key       string `json:"key"`
			Name      string `json:"name"`
			Detection struct {
				Status     string   `json:"status"`
				Evidence   []string `json:"evidence"`
				DetectedAt string   `json:"detected_at"`
			} `json:"detection"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(readBody(t, resp)), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Total != 8 {
		t.Fatalf("adapter count: %d", envelope.Total)
	}
	keys := map[string]bool{}
	for _, a := range envelope.Items {
		keys[a.Key] = true
		if a.Name == "" || a.Detection.DetectedAt == "" {
			t.Fatalf("adapter %+v", a)
		}
		switch a.Key {
		case "universal":
			if a.Detection.Status != "not_applicable" {
				t.Fatalf("universal status: %s", a.Detection.Status)
			}
		default:
			switch a.Detection.Status {
			case "detected", "not_detected", "unknown":
			default:
				t.Fatalf("adapter %s status: %s", a.Key, a.Detection.Status)
			}
		}
	}
	if !keys["pi"] || !keys["claude-code"] {
		t.Fatalf("adapter keys: %v", keys)
	}
}

// TestTargetsRESTLifecycle covers registration, dedupe, assignments,
// desired-set expansion, unassignment, and the stable error mapping.
