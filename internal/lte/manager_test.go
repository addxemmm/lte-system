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
	auto := ok
	auto.Network = ""
	if err := auto.Validate(); err != nil {
		t.Fatalf("empty network should mean auto: %v", err)
	}
	for _, invalid := range []string{"eth 0", "../../host", "0123456789abcdef", "eth0;bad"} {
		p := ok
		p.Network = invalid
		if err := p.Validate(); err == nil {
			t.Fatalf("invalid interface %q accepted", invalid)
		}
	}
}

func TestStartParams_NetName(t *testing.T) {
	base := StartParams{Band: "7", APN: "skygoapn", MCC: "001", MNC: "01", Network: "eth0"}
	ok := base
	ok.FullNetName = "SKYGO Lab"
	ok.ShortNetName = "SKYGO"
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid net names rejected: %v", err)
	}
	for _, bad := range []string{
		"a;b", `a"b`, "a'b", "#x", "a`b", "a\\b",
		"this name is way too long for a network name!!",
		"tab\there",
	} {
		p := base
		p.FullNetName = bad
		if err := p.Validate(); err == nil {
			t.Fatalf("net name %q should be rejected", bad)
		}
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

// TestRenderAll_NetName pins the operator display name rendering.
func TestRenderAll_NetName(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.ConfDir = filepath.Join(cfg.DataDir, "conf")
	cfg.LogDir = filepath.Join(cfg.DataDir, "log")
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	band, _ := Lookup("7")
	p := StartParams{Band: "7", APN: "skygoapn", MCC: "001", MNC: "01", Network: "eth0",
		FullNetName: "SKYGO Lab", ShortNetName: "SKYGO", DNS: "192.168.100.1"}
	if err := m.renderAll(p, band, "auto", "auto", 80, 40, 25); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(cfg.ConfDir, "epc_run.conf"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "full_net_name = SKYGO Lab") || !strings.Contains(s, "short_net_name = SKYGO") {
		t.Fatalf("epc_run.conf missing net names:\n%s", s)
	}
	if !strings.Contains(s, "dns_addr = 192.168.100.1") {
		t.Fatalf("epc_run.conf missing dns:\n%s", s)
	}
}

func TestRenderAll_CustomUESubnet(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.ConfDir = filepath.Join(cfg.DataDir, "conf")
	cfg.LogDir = filepath.Join(cfg.DataDir, "log")
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	band, _ := Lookup("7")
	p := StartParams{Band: "7", APN: "corp-lab", MCC: "001", MNC: "01", Network: "eth0",
		UESubnet: "10.20.30.0/24", UEAccess: "allow"}
	if err := m.renderAll(p, band, "zmq", "", 80, 40, 25); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(cfg.ConfDir, "epc_run.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "sgi_if_addr      = 10.20.30.1") {
		t.Fatalf("custom SGi gateway not rendered:\n%s", b)
	}
}

func TestValidIPv4(t *testing.T) {
	for _, ok := range []string{"8.8.8.8", "192.168.100.1", "1.2.3.4"} {
		if !validIPv4(ok) {
			t.Fatalf("%s should be valid", ok)
		}
	}
	for _, bad := range []string{"", "999.1.1.1", "1.2.3", "a.b.c.d", "01.2.3.4", "1.2.3.4.5"} {
		if validIPv4(bad) {
			t.Fatalf("%s should be invalid", bad)
		}
	}
	p := StartParams{Band: "7", APN: "a", MCC: "001", MNC: "01", Network: "eth0", DNS: "not-an-ip"}
	if err := p.Validate(); err == nil {
		t.Fatal("bad dns should be rejected")
	}
}

func TestForwardRules(t *testing.T) {
	rules := forwardRules("eth9")
	if len(rules) != 5 {
		t.Fatalf("want 5 rules, got %d", len(rules))
	}
	joined := ""
	for _, r := range rules {
		joined += r.table + " " + r.chain + " " + strings.Join(r.args, " ") + "\n"
	}
	for _, want := range []string{"filter FORWARD -i srs_spgw_sgi -s 172.16.0.0/24 -o eth9 -j ACCEPT",
		"--ctstate ESTABLISHED,RELATED", "mangle FORWARD", "TCPMSS"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("rules missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "DOCKER-USER") {
		t.Fatalf("bridge-safe rules must not depend on DOCKER-USER:\n%s", joined)
	}
	if !strings.Contains(joined, "-i srs_spgw_sgi -o srs_spgw_sgi -s 172.16.0.0/24 -d 172.16.0.0/24 -j DROP") {
		t.Fatalf("default UE isolation rule missing:\n%s", joined)
	}
	if got := strings.Join(rules[2].commandArgs("-I"), " "); got != "-w 5 -t mangle -I FORWARD -i srs_spgw_sgi -s 172.16.0.0/24 -o eth9 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu" {
		t.Fatalf("wrong iptables ordering (MSS would not apply): %s", got)
	}
}

func TestEnsureUserDBPreservesExistingAndPropagatesErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "user_db.csv")
	const existing = "existing subscriber\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureUserDB(path, dir); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != existing {
		t.Fatalf("existing database changed: %q, %v", got, err)
	}

	notDir := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(notDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureUserDB(filepath.Join(notDir, "user_db.csv"), notDir); err == nil {
		t.Fatal("seed write/setup error must be propagated")
	}
}

func TestProfile_SaveLoadOverlay(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.ConfDir = filepath.Join(cfg.DataDir, "conf")
	cfg.LogDir = filepath.Join(cfg.DataDir, "log")
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	if _, ok := m.LoadProfile(); ok {
		t.Fatal("no profile expected")
	}
	full := StartParams{Band: "7", APN: "addxLTE", MCC: "001", MNC: "01", Network: "ens33",
		FullNetName: "addxLTE", ShortNetName: "addxLTE", DNS: "192.168.100.1"}
	if err := m.SaveProfile(full); err != nil {
		t.Fatal(err)
	}
	got, ok := m.LoadProfile()
	if !ok || got.APN != "addxLTE" || got.DNS != "192.168.100.1" ||
		got.UESubnet != defaultUESubnet || got.UEAccess != defaultUEAccess {
		t.Fatalf("load failed: %v %+v", ok, got)
	}
	// Overlay: partial request inherits the rest.
	part := StartParams{Band: "40"}
	if !m.OverlayProfile(&part) {
		t.Fatal("overlay failed")
	}
	if part.APN != "addxLTE" || part.Band != "40" || part.MCC != "001" ||
		part.UESubnet != defaultUESubnet || part.UEAccess != defaultUEAccess {
		t.Fatalf("bad overlay: %+v", part)
	}
	// Corrupt file => no profile, no crash.
	_ = os.WriteFile(m.ProfilePath(), []byte("{nope"), 0o644)
	if _, ok := m.LoadProfile(); ok {
		t.Fatal("corrupt profile should be rejected")
	}
	legacy := `{"band":"7","apn":"addxLTE","mcc":"001","mnc":"01"}`
	_ = os.WriteFile(m.ProfilePath(), []byte(legacy), 0o644)
	if p, ok := m.LoadProfile(); !ok || p.Network != "auto" {
		t.Fatalf("legacy empty network should upgrade to auto: %v %+v", ok, p)
	}
}
