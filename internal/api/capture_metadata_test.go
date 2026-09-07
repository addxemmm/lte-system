package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestIncompleteCaptureFailureReportsTailWithoutClaimingMutation(t *testing.T) {
	s, cfg := testServer(t)
	if err := os.WriteFile(cfg.LogPath(cfg.PcapS1AP), []byte{0xd4, 0xc3}, 0o600); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	// Exercise only the error renderer; no extraction or job creation.
	s.writeV1CHAPFailure(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	data, ok := m["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing diagnostic data: %v", m)
	}
	capture, ok := data["capture"].(map[string]any)
	if !ok {
		t.Fatalf("missing capture metadata: %v", data)
	}
	if rec.Code != http.StatusUnprocessableEntity || data["reason"] != "capture_incomplete" || data["scan_complete"] != false {
		t.Fatalf("existing incomplete contract changed: %d %v", rec.Code, m)
	}
	if capture["tail_incomplete"] != true || capture["source_changed"] == true {
		t.Fatalf("incorrect integrity cause: %v", capture)
	}
	for _, key := range []string{"username", "password", "hash", "challenge", "response"} {
		if _, present := data[key]; present {
			t.Fatalf("credential field in diagnostics: %s", key)
		}
	}
}
