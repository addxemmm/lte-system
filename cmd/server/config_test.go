package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServerConfigDiscovery(t *testing.T) {
	t.Setenv("LTE_API_TOKEN", "")
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	if _, err := loadServerConfig("", []string{missing}); err == nil {
		t.Fatal("missing auto config silently enabled anonymous access")
	}
	path := filepath.Join(t.TempDir(), "app.yaml")
	if err := os.WriteFile(path, []byte("api_token: ''\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadServerConfig("", []string{missing, path})
	if err != nil || cfg.APIToken != "" {
		t.Fatal("explicitly configured anonymous mode failed")
	}
	if _, err := loadServerConfig(missing, []string{path}); err == nil {
		t.Fatal("missing selected config fell back")
	}
	if _, err := loadServerConfig("", []string{"\x00", path}); err == nil {
		t.Fatal("stat failure fell back")
	}
	t.Setenv("LTE_API_TOKEN", "synthetic-env-token")
	cfg, err = loadServerConfig("", []string{missing})
	if err != nil || cfg.APIToken != "synthetic-env-token" {
		t.Fatal("authenticated env-only mode failed")
	}
	if _, err := loadServerConfig(missing, nil); err == nil {
		t.Fatal("env override hid a missing selected file")
	}
}

func TestServerConfigDiscoveryKeepsFirstSelectedFile(t *testing.T) {
	t.Setenv("LTE_API_TOKEN", "")
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first.yaml"), filepath.Join(dir, "second.yaml")
	if err := os.WriteFile(first, []byte("api_token: first-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("api_token: ''\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadServerConfig("", []string{first, second})
	if err != nil || cfg.APIToken != "first-token" {
		t.Fatal("wrong configuration precedence")
	}
	if err := os.WriteFile(first, []byte("api_token: [invalid]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadServerConfig("", []string{first, second}); err == nil {
		t.Fatal("invalid first config fell back to anonymous mode")
	}
}
