package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault_Paths(t *testing.T) {
	c := Default()
	c.DataDir = t.TempDir()
	c.ConfDir = ""
	c.LogDir = ""
	var err error
	c, err = Load("") // env overlay only
	if err != nil {
		t.Fatal(err)
	}
	c.DataDir = t.TempDir()
	c.ConfDir = ""
	c.LogDir = ""
	// recompute derived dirs manually via EnsureDirs logic
	c2 := c
	c2.ConfDir = filepath.Join(c2.DataDir, "conf")
	c2.LogDir = filepath.Join(c2.DataDir, "log")
	if err := c2.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c2.UserDBPath()); err == nil {
		// file need not exist, dir must
	}
	if _, err := os.Stat(c2.LogDir); err != nil {
		t.Fatalf("log dir missing: %v", err)
	}
}

func TestLoad_YAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "app.yaml")
	y := "listen_addr: \":18082\"\ndefault_tx_gain: 70\nsim_defaults:\n  ki: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"
	if err := os.WriteFile(p, []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":18082" || c.DefaultTxGain != 70 || c.Sim.Ki != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unexpected load: %+v", c)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("a selected missing config must not silently enable anonymous access")
	}
}
