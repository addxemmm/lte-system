package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func writeLog(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "epc.log")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseEPCLog_Full(t *testing.T) {
	p := writeLog(t, "001 ESM Info: APN skygoapn\n002 Found User 001010123456780\n003 SPGW: get_new_ue_ipv4 pool ip addr 172.16.0.2\n")
	info, err := ParseEPCLog(p)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Found || info.APN != "skygoapn" || info.IMSI != "001010123456780" || info.IP != "172.16.0.2" {
		t.Fatalf("wrong parse: %+v", info)
	}
}

func TestParseEPCLog_Partial(t *testing.T) {
	p := writeLog(t, "hello world\nESM Info: APN myapn\n")
	info, err := ParseEPCLog(p)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Found || info.APN != "myapn" || info.IMSI != "" {
		t.Fatalf("wrong partial: %+v", info)
	}
}

func TestParseEPCLog_Empty(t *testing.T) {
	p := writeLog(t, "nothing here\n")
	info, err := ParseEPCLog(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Found {
		t.Fatalf("should not be found: %+v", info)
	}
}

func TestParseEPCLog_Missing(t *testing.T) {
	if _, err := ParseEPCLog(filepath.Join(t.TempDir(), "nope.log")); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseEPCLog_LastWins(t *testing.T) {
	p := writeLog(t, "ESM Info: APN first\nESM Info: APN second\nFound User 001010123456781\nFound User 001010123456782\n")
	info, _ := ParseEPCLog(p)
	if info.APN != "second" || info.IMSI != "001010123456782" {
		t.Fatalf("last-wins failed: %+v", info)
	}
}
