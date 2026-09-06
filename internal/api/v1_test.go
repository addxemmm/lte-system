package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("not JSON: %q", rec.Body.String())
	}
	if _, ok := m["request_id"]; !ok {
		t.Fatalf("missing request_id: %v", m)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing X-Request-ID header")
	}
	return m
}

func TestV1_NotFound(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("want 404 got %d", rec.Code)
	}
	if m := decodeEnvelope(t, rec); m["code"] != float64(40401) {
		t.Fatalf("want 40401: %v", m)
	}
}

func TestV1_Legacy404StaysPlain(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 404 || rec.Header().Get("Content-Type") == "application/json" {
		t.Fatalf("legacy 404 must stay plain-text: %d %q", rec.Code, rec.Body.String())
	}
}

func TestV1_MethodNotAllowed(t *testing.T) {
	s, _ := testServer(t)
	// Every documented path with a disallowed method must answer 405
	// (proves the route table and dispatcher agree).
	valid := map[string]map[string]bool{}
	for _, rt := range v1Routes {
		if valid[rt.path] == nil {
			valid[rt.path] = map[string]bool{}
		}
		valid[rt.path][rt.method] = true
	}
	probe := map[string]string{ // one path per group is enough
		"/api/v1/cell": "PATCH", "/api/v1/ue": "POST",
		"/api/v1/crack/jobs": "GET", "/api/v1/crack/result": "DELETE",
		"/api/v1/config/subscribers": "GET", "/api/v1/config/wordlist": "DELETE",
		"/api/v1/captures/lte-data": "POST", "/api/v1/simcards": "GET",
		"/api/v1/profile": "POST", "/api/v1/health": "POST",
	}
	for path, method := range probe {
		if valid[path][method] {
			t.Fatalf("probe method %s is actually valid for %s", method, path)
		}
		req := httptest.NewRequest(method, path, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != 405 || decodeEnvelope(t, rec)["code"] != float64(40501) {
			t.Fatalf("%s %s: want 405 got %d (%s)", method, path, rec.Code, rec.Body.String())
		}
	}
}

func TestV1_StartValidation(t *testing.T) {
	s, _ := testServer(t)
	// Malformed.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cell", bytes.NewBufferString("{bad"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 400 || decodeEnvelope(t, rec)["code"] != float64(40001) {
		t.Fatalf("malformed: %d %s", rec.Code, rec.Body.String())
	}
	// Unknown band must NOT silently fall back (legacy did).
	req = httptest.NewRequest(http.MethodPost, "/api/v1/cell",
		bytes.NewBufferString(`{"band":"400","apn":"a","mcc":"001","mnc":"01","network":"eth0"}`))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	m := decodeEnvelope(t, rec)
	if rec.Code != 422 || m["code"] != float64(42201) {
		t.Fatalf("bad band: %d %v", rec.Code, m)
	}
	// Bad gain + bad n_prb pin the whitelist.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/cell",
		bytes.NewBufferString(`{"band":"7","apn":"a","mcc":"001","mnc":"01","network":"eth0","tx_gain":999,"n_prb":99}`))
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	m = decodeEnvelope(t, rec)
	data, _ := m["data"].(map[string]any)
	errs, _ := data["errors"].([]any)
	if rec.Code != 422 || len(errs) != 2 {
		t.Fatalf("want 2 field errors: %d %v", rec.Code, m)
	}
}

func TestV1_StopIdempotent(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cell", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	m := decodeEnvelope(t, rec)
	if rec.Code != 200 || m["code"] != float64(0) {
		t.Fatalf("idle stop should succeed: %d %v", rec.Code, m)
	}
}

func TestV1_UENotRunning(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ue", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 412 || decodeEnvelope(t, rec)["code"] != float64(41201) {
		t.Fatalf("want 412: %d %s", rec.Code, rec.Body.String())
	}
}

func TestV1_Captures(t *testing.T) {
	s, cfg := testServer(t)
	// Missing file -> 404.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/captures/lte-data", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("want 404 got %d", rec.Code)
	}
	// Unknown id -> 404.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/captures/nope", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("want 404 got %d", rec.Code)
	}
	// Present file -> download (string id and legacy numeric alias).
	if err := os.WriteFile(cfg.LogPath(cfg.PcapLTEData), []byte("pcap"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"lte-data", "0"} {
		req = httptest.NewRequest(http.MethodGet, "/api/v1/captures/"+id, nil)
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != 200 || rec.Body.String() != "pcap" {
			t.Fatalf("id %s: %d %q", id, rec.Code, rec.Body.String())
		}
	}
}

func TestV1_Uploads(t *testing.T) {
	s, cfg := testServer(t)
	// Missing field -> 422.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/subscribers", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 422 {
		t.Fatalf("want 422 got %d %s", rec.Code, rec.Body.String())
	}
	// Happy path carries the restart note.
	buf.Reset()
	mw = multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("userdb", "user_db.csv")
	_, _ = fw.Write([]byte("ue0,mil,001010123456789,aaa,opc,bbb,8000,000000001234,7,dynamic\n"))
	_ = mw.Close()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/config/subscribers", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	m := decodeEnvelope(t, rec)
	if rec.Code != 200 {
		t.Fatalf("upload: %d %v", rec.Code, m)
	}
	if m["data"].(map[string]any)["note"] == nil {
		t.Fatalf("restart note missing: %v", m)
	}
	if _, err := os.Stat(cfg.UserDBPath()); err != nil {
		t.Fatal(err)
	}
}

func TestV1_SIMValidation(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/simcards", bytes.NewBufferString(`{"mcc":"001"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 422 {
		t.Fatalf("want 422 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestV1_HealthAndProfile(t *testing.T) {
	s, _ := testServer(t)
	for _, p := range []string{"/api/v1/health", "/api/v1/profile", "/api/v1/cell"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", p, rec.Code, rec.Body.String())
		}
		decodeEnvelope(t, rec)
	}
}

func TestV1_Auth(t *testing.T) {
	t.Setenv("LTE_API_TOKEN", "secret")
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("want 401 got %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200 got %d", rec.Code)
	}
	// Legacy routes are covered by the same gate.
	req = httptest.NewRequest(http.MethodPost, "/stop", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("legacy must also require token: %d", rec.Code)
	}
}
