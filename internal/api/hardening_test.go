package api

import (
	"bytes"
	"encoding/json"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func multipartBody(t *testing.T, field, name string, payload []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	f, err := mw.CreateFormFile(field, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, mw.FormDataContentType()
}

func TestJSONBodiesAreSingleObjects(t *testing.T) {
	s, cfg := testServer(t)

	for _, body := range []string{"null", `[]`, `{}`, `{} {}`, `{} trailing`} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/simcards", strings.NewReader(body))
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		want := http.StatusBadRequest
		if body == `{}` {
			want = http.StatusUnprocessableEntity
		}
		if rec.Code != want {
			t.Fatalf("v1 body %q: want %d, got %d %s", body, want, rec.Code, rec.Body.String())
		}
	}

	// The v1 SIM endpoint intentionally ignores unknown fields.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/simcards", strings.NewReader(`{"future":true}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("v1 SIM unknown field policy changed: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(cfg.UserDBPath()); !os.IsNotExist(err) {
		t.Fatalf("rejected JSON must not update user DB: %v", err)
	}
}

func TestV1JSONSizeBoundary(t *testing.T) {
	s, _ := testServer(t)
	for _, tc := range []struct {
		path  string
		limit int
	}{
		{path: "/api/v1/cell", limit: 1 << 20},
		{path: "/api/v1/simcards", limit: 64 << 10},
	} {
		exact := "{}" + strings.Repeat(" ", tc.limit-2)
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(exact))
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s exact limit: want validation 422, got %d %s", tc.path, rec.Code, rec.Body.String())
		}

		req = httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(exact+" "))
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("%s limit+1: want 413, got %d %s", tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestV1UploadLimitsPreserveOldFile(t *testing.T) {
	s, cfg := testServer(t)
	s.cfg.MaxUploadBytes = 64
	dst := cfg.WordlistPath()
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	exact := bytes.Repeat([]byte("e"), int(s.cfg.MaxUploadBytes))
	body, contentType := multipartBody(t, "wordlist", "wordlist.list", exact)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/wordlist", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("exact limit: %d %s", rec.Code, rec.Body.String())
	}

	for _, payload := range [][]byte{
		bytes.Repeat([]byte("x"), int(s.cfg.MaxUploadBytes+1)),
		bytes.Repeat([]byte("y"), int(s.cfg.MaxUploadBytes+(1<<20)+1)),
	} {
		body, contentType = multipartBody(t, "wordlist", "wordlist.list", payload)
		req = httptest.NewRequest(http.MethodPost, "/api/v1/config/wordlist", body)
		req.Header.Set("Content-Type", contentType)
		rec = httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("oversize %d: want 413, got %d %s", len(payload), rec.Code, rec.Body.String())
		}
		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, exact) {
			t.Fatalf("failed upload replaced old file: got %d bytes", len(got))
		}
	}
}

func TestConcurrentUploadsRemainComplete(t *testing.T) {
	s, cfg := testServer(t)
	s.cfg.MaxUploadBytes = 256 << 10
	const requests = 12
	payloads := make([][]byte, requests)
	var wg sync.WaitGroup
	errs := make(chan string, requests)
	for i := range payloads {
		payloads[i] = bytes.Repeat([]byte{byte('A' + i)}, 128<<10)
		body, contentType := multipartBody(t, "wordlist", "wordlist.list", payloads[i])
		requestBody := append([]byte(nil), body.Bytes()...)
		wg.Add(1)
		go func(body []byte, contentType string) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/config/wordlist", bytes.NewReader(body))
			req.Header.Set("Content-Type", contentType)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				errs <- rec.Body.String()
				return
			}
			var response Envelope
			if json.Unmarshal(rec.Body.Bytes(), &response) != nil || response.Code != CodeOK {
				errs <- rec.Body.String()
			}
		}(requestBody, contentType)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent upload failed: %s", err)
	}

	got, err := os.ReadFile(cfg.WordlistPath())
	if err != nil {
		t.Fatal(err)
	}
	complete := false
	for _, payload := range payloads {
		if bytes.Equal(got, payload) {
			complete = true
			break
		}
	}
	if !complete {
		t.Fatalf("final upload is truncated or mixed: %d bytes", len(got))
	}
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(cfg.WordlistPath()), ".upload-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("temporary uploads not cleaned up: %v", leftovers)
	}
}

func TestMiddlewareAuditsAuthAndPanics(t *testing.T) {
	oldWriter, oldFlags, oldPrefix := log.Writer(), log.Flags(), log.Prefix()
	var logs bytes.Buffer
	log.SetOutput(&logs)
	log.SetFlags(0)
	log.SetPrefix("")
	defer func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	}()

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	rec := httptest.NewRecorder()
	chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("unauthorized request reached handler")
	}), "secret").ServeHTTP(rec, req)
	authResponse := decodeEnvelope(t, rec)
	authRID := rec.Header().Get("X-Request-ID")
	if rec.Code != http.StatusUnauthorized || authResponse["request_id"] != authRID ||
		!strings.Contains(logs.String(), "rid="+authRID+" GET /secret -> 401") {
		t.Fatalf("401 was not audited: status=%d logs=%q", rec.Code, logs.String())
	}

	logs.Reset()
	var panicRID string
	req = httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec = httptest.NewRecorder()
	chain(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		panicRID = RequestID(r)
		panic("boom")
	}), "").ServeHTTP(rec, req)
	response := decodeEnvelope(t, rec)
	if rec.Code != http.StatusInternalServerError || panicRID == "" || response["request_id"] != panicRID {
		t.Fatalf("panic response lacks request id: status=%d rid=%q body=%v", rec.Code, panicRID, response)
	}
	if !strings.Contains(logs.String(), "rid="+panicRID+" panic recovered") ||
		!strings.Contains(logs.String(), "rid="+panicRID+" GET /panic -> 500") {
		t.Fatalf("panic was not audited: %q", logs.String())
	}

	logs.Reset()
	req = httptest.NewRequest(http.MethodGet, "/partial", nil)
	rec = httptest.NewRecorder()
	chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("partial"))
		panic("after write")
	}), "").ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted || rec.Body.String() != "partial" {
		t.Fatalf("panic corrupted committed response: %d %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logs.String(), "GET /partial -> 202") {
		t.Fatalf("committed panic status not audited: %q", logs.String())
	}
}
