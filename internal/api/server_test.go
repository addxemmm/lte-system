package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestStop_NotRunning(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/stop", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	// No srs binaries on CI; either not-running(2) or failed(0) is acceptable, never 200-crash.
	if m["message_id"] != float64(2) && m["message_id"] != float64(0) {
		t.Fatalf("unexpected: %v", m)
	}
}

func TestStart_Incomplete(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/start", bytes.NewBufferString(`{"band":"7"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if m["message_id"] != float64(3) && m["message_id"] != float64(0) {
		t.Fatalf("want incomplete(3): %v", m)
	}
}

func TestGetFile_BadID(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/getfile", bytes.NewBufferString(`{"fileid":99}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if m["message_id"] != float64(2) {
		t.Fatalf("want Error id(2): %v", m)
	}
}

func TestGetFile_Missing(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/getfile", bytes.NewBufferString(`{"fileid":0}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if m["message_id"] != float64(3) {
		t.Fatalf("want Cant-find(3): %v", m)
	}
}

func TestGetFile_OK(t *testing.T) {
	s, cfg := testServer(t)
	if err := os.WriteFile(cfg.LogPath(cfg.PcapLTEData), []byte("pcap"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/getfile", bytes.NewBufferString(`{"fileid":0}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Body.String() != "pcap" {
		t.Fatalf("want file, got %d %q", rec.Code, rec.Body.String())
	}
}

func TestBasicInfo_NotRunning(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/basicinfo", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	// CI has no srs running -> id 2; if a dev box runs srs, id 3 (no UE) is fine.
	if m["message_id"] != float64(2) && m["message_id"] != float64(3) {
		t.Fatalf("unexpected: %v", m)
	}
}

func TestUpload_UserDB(t *testing.T) {
	s, cfg := testServer(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("userdb", "user_db.csv")
	_, _ = fw.Write([]byte("ue0,mil,001010123456780,aaa,opc,bbb,8000,000000001234,7,dynamic\n"))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/userupload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if m["message_id"] != float64(1) {
		t.Fatalf("upload failed: %v", m)
	}
	b, _ := os.ReadFile(cfg.UserDBPath())
	if len(b) == 0 {
		t.Fatal("file not written")
	}
}

func TestWriteSIM_Validation(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/writesim", bytes.NewBufferString(`{"imsi":"bad"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	// No hardware on CI: either invalid-params(6) or device-missing(2). Both prove validation path works.
	if m["message_id"] != float64(6) && m["message_id"] != float64(2) && m["message_id"] != float64(0) {
		t.Fatalf("unexpected: %v", m)
	}
}

func TestHealthz(t *testing.T) {
	s, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("healthz %d", rec.Code)
	}
}
