package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/addxemmm/lte-system/internal/api"
	"github.com/addxemmm/lte-system/internal/config"
	"github.com/addxemmm/lte-system/internal/lte"
)

func TestConsoleGatewaySecurityAndScope(t *testing.T) {
	for _, tc := range []struct {
		method, path, origin, marker, auth string
		want                               int
	}{
		{"GET", "/api/v1/profile", "", "", "", 401},
		{"GET", "/api/v1/profile", "", "", "Bearer fixture", 204},
		{"GET", "/api/v1/profile", "https://other.invalid", "", "Bearer fixture", 403},
		{"POST", "/api/v1/cell", "", "", "Bearer fixture", 403},
		{"POST", "/api/v1/cell", "http://example.com", "1", "Bearer fixture", 204},
		{"POST", "/api/v1/cell", "http://other.invalid", "1", "Bearer fixture", 403},
		{"POST", "/api/v1/crack/jobs", "", "1", "Bearer fixture", 404},
		{"GET", "/api/v1/captures/lte-data", "", "", "Bearer fixture", 404},
		{"POST", "/api/v1/subscribers", "", "1", "Bearer fixture", 404},
	} {
		t.Run(tc.method+tc.path+tc.origin+tc.marker+tc.auth, func(t *testing.T) {
			backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fixture" {
					w.WriteHeader(401)
					return
				}
				w.WriteHeader(204)
			})
			h := Handler(backend, PublicConfig{}, "example.com")
			r := httptest.NewRequest(tc.method, "http://example.com"+tc.path, nil)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("X-LTE-UI", tc.marker)
			r.Header.Set("Authorization", tc.auth)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d wanted %d", w.Code, tc.want)
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("unsafe caching or CORS")
			}
		})
	}
}

func TestConsoleAndDirectAPIShareAuthentication(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.ConfDir = cfg.DataDir
	cfg.LogDir = cfg.DataDir
	cfg.APIToken = "fixture-token"
	backend := api.New(cfg, lte.New(cfg)).Handler()
	console := Handler(backend, PublicConfig{AuthRequired: true})
	for _, handler := range []http.Handler{backend, console} {
		for _, token := range []string{"", "Bearer wrong", "Bearer fixture-token"} {
			r := httptest.NewRequest("GET", "http://127.0.0.1/api/v1/profile", nil)
			r.Header.Set("Authorization", token)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			want := http.StatusUnauthorized
			if token == "Bearer fixture-token" {
				want = http.StatusOK
			}
			if w.Code != want || w.Header().Get("X-Request-ID") == "" {
				t.Fatalf("auth status %d, wanted %d", w.Code, want)
			}
			if strings.Contains(w.Body.String(), cfg.APIToken) {
				t.Fatal("response leaked token")
			}
		}
	}
}

func TestConsoleStaticAndPublicMetadata(t *testing.T) {
	h := Handler(http.NotFoundHandler(), PublicConfig{Version: "2.1", APIExposed: false, APIPort: 8081, UIPort: 18081, AuthRequired: true}, "example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/ui-config.json", nil))
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 5 || body["auth_required"] != true || body["api_exposed"] != false ||
		body["api_port"] != float64(8081) || body["ui_port"] != float64(18081) {
		t.Fatal("wrong public metadata")
	}
	for _, p := range []string{"/", "/app.js", "/styles.css"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "script-src 'self'") {
			t.Fatalf("asset %s status %d", p, w.Code)
		}
	}
	for _, p := range []string{"/../configs/app.yaml", "/.env", "/missing.js", "/static/"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		if w.Code != 404 {
			t.Fatalf("unexpected static path %s status %d", p, w.Code)
		}
	}
}

func TestConsoleRejectsRebindingHostAndCrossSite(t *testing.T) {
	h := Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), PublicConfig{})
	for _, tc := range []struct {
		host, site string
		status     int
	}{
		{"rebind.invalid:18081", "same-origin", 403},
		{"127.0.0.1:18081", "cross-site", 403},
		{"192.0.2.10:18081", "same-origin", 204},
		{"[::1]:18081", "same-origin", 204},
		{"localhost:18081", "same-origin", 204},
	} {
		r := httptest.NewRequest("POST", "http://"+tc.host+"/api/v1/cell", strings.NewReader("{}"))
		r.Header.Set("Origin", "http://"+tc.host)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		r.Header.Set("X-LTE-UI", "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s => %d", tc.host, tc.site, w.Code)
		}
		if w.Code == 403 && w.Header().Get("X-Request-ID") == "" {
			t.Fatal("missing rejection request ID")
		}
	}
}
