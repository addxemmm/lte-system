package api

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/addxemmm/lte-system/internal/config"
	"github.com/addxemmm/lte-system/internal/lte"
)

func TestConfiguredTokenGate(t *testing.T) {
	for _, tc := range []struct {
		name, token string
		headers     []string
		want        int
	}{
		{"open", "", nil, 204},
		{"open-ignores-bad-header", "", []string{"bad"}, 204},
		{"blank-config", " \t", nil, 204},
		{"missing", "fixed-token", nil, 401},
		{"wrong", "fixed-token", []string{"Bearer wrong"}, 401},
		{"valid", "fixed-token", []string{"Bearer fixed-token"}, 204},
		{"scheme-case", "fixed-token", []string{"bearer fixed-token"}, 204},
		{"token-case", "fixed-token", []string{"Bearer FIXED-TOKEN"}, 401},
		{"basic", "fixed-token", []string{"Basic fixed-token"}, 401},
		{"extra-field", "fixed-token", []string{"Bearer fixed-token extra"}, 401},
		{"empty-bearer", "fixed-token", []string{"Bearer "}, 401},
		{"duplicate", "fixed-token", []string{"Bearer fixed-token", "Bearer fixed-token"}, 401},
		{"comma-joined", "fixed-token", []string{"Bearer fixed-token, Bearer fixed-token"}, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			h := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(204) }), tc.token)
			r := httptest.NewRequest(http.MethodGet, "/api/v1/profile?token=fixed-token", nil)
			r.AddCookie(&http.Cookie{Name: "token", Value: "fixed-token"})
			for _, v := range tc.headers {
				r.Header.Add("Authorization", v)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want || called != (tc.want == 204) {
				t.Fatalf("status=%d called=%v", w.Code, called)
			}
			if tc.want == 401 {
				m := decodeEnvelope(t, w)
				if m["code"] != float64(CodeUnauthorized) || w.Header().Get("WWW-Authenticate") != "Bearer" {
					t.Fatal("wrong authentication error contract")
				}
			}
		})
	}
}

func TestTokenConfigurationLoadedOncePerServer(t *testing.T) {
	t.Setenv("LTE_API_TOKEN", "")
	path := filepath.Join(t.TempDir(), "app.yaml")
	if err := os.WriteFile(path, []byte("api_token: file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DataDir = t.TempDir()
	cfg.ConfDir = filepath.Join(cfg.DataDir, "conf")
	cfg.LogDir = filepath.Join(cfg.DataDir, "log")
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	s := New(cfg, lte.New(cfg))
	t.Setenv("LTE_API_TOKEN", "changed-env-token")
	if err := os.WriteFile(path, []byte("api_token: ''\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token string
		want  int
	}{{"", 401}, {"changed-env-token", 401}, {"file-token", 200}} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/profile", nil)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("status=%d want=%d", w.Code, tc.want)
		}
	}
}

func TestAllRoutesRequireConfiguredTokenBeforeDispatch(t *testing.T) {
	s, _ := testServer(t)
	s.cfg.APIToken = "fixed-token"
	for _, route := range v1Routes {
		r := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("route %s %s bypassed auth", route.method, route.path)
		}
	}
	for _, path := range []string{"/", "/unknown", "/api/v1/unknown"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodOptions, path, nil))
		if w.Code != 401 {
			t.Fatalf("path %s bypassed auth", path)
		}
	}
}

func TestAuthDoesNotLogOrReturnTokens(t *testing.T) {
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)
	h := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), "configured-secret")
	r := httptest.NewRequest(http.MethodGet, "/api/v1/profile?token=query-secret", nil)
	r.Header.Set("Authorization", "Bearer supplied-secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	for _, secret := range []string{"configured-secret", "supplied-secret", "query-secret"} {
		if strings.Contains(logs.String()+w.Body.String(), secret) {
			t.Fatal("auth leaked a token")
		}
	}
}
