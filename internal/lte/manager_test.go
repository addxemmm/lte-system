package lte

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/addxemmm/lte-system/internal/config"
)

func TestStartParams_Validate(t *testing.T) {
	ok := StartParams{Band: "7", APN: "skygoapn", MCC: "001", MNC: "01", Network: "eth0"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid rejected: %v", err)
	}
	bad := StartParams{Band: "7", APN: "", MCC: "001", MNC: "01", Network: "eth0"}
	if err := bad.Validate(); err == nil {
		t.Fatal("expected incomplete error")
	}
	badMCC := StartParams{Band: "7", APN: "a", MCC: "01", MNC: "01", Network: "eth0"}
	if err := badMCC.Validate(); err == nil {
		t.Fatal("expected mcc error")
	}
	inject := StartParams{Band: "7", APN: "a; rm -rf /", MCC: "001", MNC: "01", Network: "eth0"}
	if err := inject.Validate(); err == nil {
		t.Fatal("expected apn injection error")
	}
}

// TestRenderAll_RR pins that rr.conf is rendered per-band with explicit
// ul_earfcn (the TDD workaround): band 40 must get dl==ul==39150.
func TestRenderAll_RR(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.ConfDir = filepath.Join(cfg.DataDir, "conf")
	cfg.LogDir = filepath.Join(cfg.DataDir, "log")
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	band, _ := Lookup("40")
	p := StartParams{Band: "40", APN: "skygoapn", MCC: "001", MNC: "01", Network: "eth0"}
	if err := m.renderAll(p, band, "uhd", "auto", 80, 40, 50); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(cfg.ConfDir, "rr.conf"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "dl_earfcn = 39150;") || !strings.Contains(s, "ul_earfcn = 39150;") {
		t.Fatalf("rr.conf missing band-40 earfcns:\n%s", s[:500])
	}
	if strings.Contains(s, "{{") {
		t.Fatal("unrendered template placeholder left in rr.conf")
	}
	enb, err := os.ReadFile(filepath.Join(cfg.ConfDir, "enb_run.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(enb), "rb_config = ") || strings.Contains(string(enb), "drb_config") {
		t.Fatal("enb_run.conf must use rb_config (srsRAN_4G renamed drb_config)")
	}
}
