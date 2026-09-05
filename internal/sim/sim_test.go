package sim

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/addxemmm/lte-system/internal/config"
)

func TestResolve_Defaults(t *testing.T) {
	cfg := config.Default()
	r, err := Resolve(WriteRequest{IMSI: "001010123456789"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r.MCC != "001" || r.MNC != "01" || r.Ki != cfg.Sim.Ki || r.OPValue != cfg.Sim.OPc || !r.UseOPc {
		t.Fatalf("defaults wrong: %+v", r)
	}
	if r.Auth != "mil" || r.Card != "testsim" {
		t.Fatalf("auth/card wrong: %+v", r)
	}
}

func TestResolve_CustomKeys(t *testing.T) {
	cfg := config.Default()
	op := "ffffffffffffffffffffffffffffffff"
	r, err := Resolve(WriteRequest{IMSI: "460001234567890", Ki: op, OP: op, Auth: "xor", AMF: "9001", ACC: "0200", SQN: "000000001234"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r.UseOPc || r.OPType != "op" || r.MCC != "460" {
		t.Fatalf("custom wrong: %+v", r)
	}
}

func TestResolve_Rejects(t *testing.T) {
	cfg := config.Default()
	for _, req := range []WriteRequest{
		{IMSI: "short"},
		{IMSI: "001010123456789", Ki: "zz"},
		{IMSI: "001010123456789", OP: strings.Repeat("a", 32), OPc: strings.Repeat("b", 32)},
		{IMSI: "001010123456789", Auth: "comp128"},
		{IMSI: "001010123456789", AMF: "xyz"},
	} {
		if _, err := Resolve(req, cfg); err == nil {
			t.Fatalf("should reject %+v", req)
		}
	}
}

func TestProgArgs_NoShell(t *testing.T) {
	cfg := config.Default()
	r, _ := Resolve(WriteRequest{IMSI: "001010123456789"}, cfg)
	argv := r.ProgArgs("/opt/pysim")
	joined := strings.Join(argv, " ")
	for _, want := range []string{"-x 001", "-y 01", "-i 001010123456789", "-k 00112233445566778899aabbccddeeff", "-t testsim"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
}

func TestAddUser(t *testing.T) {
	cfg := config.Default()
	dir := t.TempDir()
	p := filepath.Join(dir, "user_db.csv")
	seed := "# comment\nue0,mil,001010123456780,00112233445566778899aabbccddeeff,opc,63bfa50ee6523365ff14c1f45f88737d,8001,000000003521,7,dynamic\n"
	if err := os.WriteFile(p, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	r, _ := Resolve(WriteRequest{IMSI: "001010123456789", SQN: "000000001234"}, cfg)
	st, err := AddUser(p, r)
	if err != nil || st != "added" {
		t.Fatalf("add: %v %s", err, st)
	}
	st2, _ := AddUser(p, r)
	if st2 != "exists" {
		t.Fatalf("dup should be exists, got %s", st2)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "001010123456789") || !strings.Contains(string(b), "ue1,") {
		t.Fatalf("bad csv:\n%s", b)
	}
}

func TestSummarize(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "user_db.csv")
	seed := "# comment\nue3,mil,001012333333333,KEY,opc,OPC,8001,000000001234,7,dynamic\nbadline\n"
	if err := os.WriteFile(p, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	ues, err := Summarize(p)
	if err != nil || len(ues) != 1 {
		t.Fatalf("want 1 entry: %v %+v", err, ues)
	}
	if ues[0].IMSI != "001012333333333" || ues[0].Name != "ue3" || ues[0].Auth != "mil" {
		t.Fatalf("wrong entry: %+v", ues[0])
	}
	if ues, err := Summarize(filepath.Join(dir, "missing.csv")); err != nil || len(ues) != 0 {
		t.Fatalf("missing file should give empty list: %v %+v", err, ues)
	}
}
