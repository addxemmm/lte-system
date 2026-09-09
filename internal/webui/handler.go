// Package webui serves embedded console assets and a same-origin management gateway.
package webui

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
)

//go:embed static
var assets embed.FS

// PublicConfig contains presentation metadata only, never configuration secrets.
type PublicConfig struct {
	Version      string `json:"version"`
	APIExposed   bool   `json:"api_exposed"`
	APIPort      int    `json:"api_port"`
	UIPort       int    `json:"ui_port"`
	AuthRequired bool   `json:"auth_required"`
}

func sameOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return false
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		return u.Scheme == scheme && strings.EqualFold(u.Host, r.Host)
	}
	return true
}

// Matching Origin alone does not prevent DNS rebinding. Console authorities
// therefore default to literal IPs/localhost; named hosts need an explicit list.
func trustedHost(authority string, allowed []string) bool {
	host := authority
	if h, _, err := net.SplitHostPort(authority); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || net.ParseIP(strings.Trim(host, "[]")) != nil {
		return true
	}
	for _, candidate := range allowed {
		if host == strings.TrimSuffix(strings.ToLower(strings.TrimSpace(candidate)), ".") {
			return true
		}
	}
	return false
}

func allowedAPI(r *http.Request) bool {
	p := r.URL.Path
	if p == "/api/v1/cell" {
		return r.Method == http.MethodGet || r.Method == http.MethodPost || r.Method == http.MethodDelete
	}
	if r.Method != http.MethodGet {
		return false
	}
	switch p {
	case "/api/v1/network", "/api/v1/ues", "/api/v1/subscribers", "/api/v1/profile", "/api/v1/health", "/api/v1/diagnostics/connectivity":
		return true
	}
	for _, prefix := range []string{"/api/v1/ues/", "/api/v1/subscribers/"} {
		if strings.HasPrefix(p, prefix) {
			id := strings.TrimPrefix(p, prefix)
			return id != "" && !strings.Contains(id, "/")
		}
	}
	return false
}

func failure(w http.ResponseWriter, status int, message string) {
	var random [16]byte
	_, _ = rand.Read(random[:])
	id := hex.EncodeToString(random[:])
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", id)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": status*100 + 1, "message": message, "request_id": id})
}

// Handler never starts services or runs commands. Token authentication remains
// inside apiHandler; the gateway does not inject a server-side credential.
func Handler(apiHandler http.Handler, cfg PublicConfig, allowedHosts ...string) http.Handler {
	static, _ := fs.Sub(assets, "static")
	files := http.FileServer(http.FS(static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if !trustedHost(r.Host, allowedHosts) {
			w.Header().Set("Cache-Control", "no-store")
			failure(w, 403, "console host not allowed; configure ui_allowed_hosts for named hosts")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			if !sameOrigin(r) {
				failure(w, 403, "cross-origin console request rejected")
				return
			}
			if !allowedAPI(r) {
				failure(w, 404, "endpoint not available through console gateway")
				return
			}
			if r.Method != http.MethodGet && r.Header.Get("X-LTE-UI") != "1" {
				failure(w, 403, "console request header required")
				return
			}
			apiHandler.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/ui-config.json" {
			w.Header().Set("Cache-Control", "no-store")
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", "GET")
				failure(w, 405, "method not allowed")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(cfg)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", 405)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		if !fs.ValidPath(name) || path.Clean(name) != name {
			http.NotFound(w, r)
			return
		}
		entry, err := fs.Stat(static, name)
		if err != nil || entry.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
}
