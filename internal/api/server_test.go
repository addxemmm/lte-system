package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/addxemmm/lte-system/internal/config"
	"github.com/addxemmm/lte-system/internal/lte"
)

func testServer(t *testing.T) (*Server, config.Config) {
	t.Helper()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.ConfDir = filepath.Join(cfg.DataDir, "conf")
	cfg.LogDir = filepath.Join(cfg.DataDir, "log")
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	return New(cfg, lte.New(cfg)), cfg
}

func TestRemovedLegacyRoutesReturnStandard404(t *testing.T) {
	s, _ := testServer(t)
	paths := []string{
		"/start", "/stop", "/basicinfo", "/crackapn", "/getcrackresult",
		"/userupload", "/passwordupload", "/getfile", "/writesim",
		"/healthz", "/status", "/profile", "/api/v1/ue",
	}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: want 404 got %d", path, rec.Code)
		}
		var body Envelope
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code != CodeNotFound || body.RequestID == "" {
			t.Fatalf("%s: non-standard response: %s", path, rec.Body.String())
		}
	}
}

func TestV1ProfileContainsNoSubscriberRows(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profile", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("profile: %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	data := body["data"].(map[string]any)
	if _, exists := data["ues"]; exists {
		t.Fatalf("profile duplicates subscriber resources: %v", data)
	}
}
