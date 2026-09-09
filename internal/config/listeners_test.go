package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConsoleListenerDefaultsAndOverrides(t *testing.T) {
	t.Setenv("LTE_UI_LISTEN", "")
	t.Setenv("LTE_LISTEN", "")
	t.Setenv("LTE_EXPOSE_API", "")
	cfg, err := Load("")
	if err != nil || cfg.ExposeAPI || cfg.UIListenAddr != ":18081" || cfg.ListenAddr != ":8081" {
		t.Fatal("unexpected listener defaults")
	}
	p := filepath.Join(t.TempDir(), "app.yaml")
	if err := os.WriteFile(p, []byte("ui_listen_addr: ':9090'\nlisten_addr: ':9091'\nexpose_api: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(p)
	if err != nil || !cfg.ExposeAPI || cfg.UIListenAddr != ":9090" || cfg.ListenAddr != ":9091" {
		t.Fatal("file listener configuration failed")
	}
	t.Setenv("LTE_EXPOSE_API", "false")
	t.Setenv("LTE_UI_LISTEN", "127.0.0.1:8081")
	t.Setenv("LTE_LISTEN", "127.0.0.1:18082")
	cfg, err = Load(p)
	if err != nil || cfg.ExposeAPI || cfg.UIListenAddr != "127.0.0.1:8081" || cfg.ListenAddr != "127.0.0.1:18082" {
		t.Fatal("explicit false override failed")
	}
	t.Setenv("LTE_EXPOSE_API", "typo")
	if _, err := Load(p); err == nil {
		t.Fatal("invalid exposure option silently accepted")
	}
}

func TestListenerPortValidation(t *testing.T) {
	for _, value := range []string{"", ":0", ":65536", "http://localhost:18081", "localhost", "localhost:api"} {
		if _, err := ListenPort(value); err == nil {
			t.Fatalf("bad port accepted: %q", value)
		}
	}
	c := Default()
	c.ListenAddr = ":18081"
	c.ExposeAPI = true
	if c.ValidateListeners() == nil {
		t.Fatal("port collision accepted")
	}
	c.ExposeAPI = false
	if err := c.ValidateListeners(); err != nil {
		t.Fatal("disabled API unnecessarily reserves port")
	}
}
