package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPITokenConfigPrecedence(t *testing.T) {
	for _, tc := range []struct{ name, yaml, env, want string }{
		{"omitted", "listen_addr: ':8081'", "", ""},
		{"empty", "api_token: ''", "", ""},
		{"whitespace", "api_token: '   '", "", ""},
		{"file", "api_token: '  file-token  '", "", "file-token"},
		{"empty-env-preserves-file", "api_token: file-token", " \t ", "file-token"},
		{"environment", "api_token: ''", " env-token ", "env-token"},
		{"environment-override", "api_token: file-token", "env-token", "env-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LTE_API_TOKEN", tc.env)
			path := filepath.Join(t.TempDir(), "app.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.APIToken != tc.want {
				t.Fatal("unexpected effective token")
			}
		})
	}
}

func TestAPITokenDefaultAndEnvironmentOnly(t *testing.T) {
	if Default().APIToken != "" {
		t.Fatal("default authentication must be optional")
	}
	t.Setenv("LTE_API_TOKEN", "env-only-token")
	cfg, err := Load("")
	if err != nil || cfg.APIToken != "env-only-token" {
		t.Fatal("environment-only configuration failed")
	}
}

func TestAPITokenNotSerializedAsJSON(t *testing.T) {
	cfg := Default()
	cfg.APIToken = "synthetic-config-secret"
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), cfg.APIToken) || strings.Contains(string(b), "APIToken") {
		t.Fatal("JSON exposed API token")
	}
}

func TestConfigParseErrorDoesNotExposeToken(t *testing.T) {
	for _, content := range []string{
		"api_token: [synthetic-secret]\n",
		"api_token: synthetic-secret\nlisten_addr: [synthetic-secret]\n",
	} {
		path := filepath.Join(t.TempDir(), "app.yaml")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := Load(path)
		if err == nil {
			t.Fatal("invalid configuration accepted")
		}
		if strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatal("error exposed configuration content")
		}
	}
}
