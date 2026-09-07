package crack

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// realisticCHAP mimics `tshark -V` output for two CHAP exchanges: an old
// complete one (id 0x03) and a newer complete one (id 0x05). The parser must
// return the NEWER handshake, not a mix.
const realisticCHAP = `Point-to-Point Protocol
    PPP Challenge Handshake Authentication Protocol
        Code: Challenge (1)
        Identifier: 0x03 (3)
        Length: 32
        Challenge Value: 11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff:00
        Name: olduser
Point-to-Point Protocol
    PPP Challenge Handshake Authentication Protocol
        Code: Response (2)
        Identifier: 0x03 (3)
        Length: 49
        Response Value: aa:bb:cc:dd:ee:ff:00:11:22:33:44:55:66:77:88:99
        Name: olduser
Point-to-Point Protocol
    PPP Challenge Handshake Authentication Protocol
        Code: Challenge (1)
        Identifier: 0x05 (5)
        Length: 32
        Challenge Value: 01:23:45:67:89:ab:cd:ef:01:23:45:67:89:ab:cd:ef
        Name: mi6test
Point-to-Point Protocol
    PPP Challenge Handshake Authentication Protocol
        Code: Response (2)
        Identifier: 0x05 (5)
        Length: 49
        Response Value: fe:dc:ba:98:76:54:32:10:fe:dc:ba:98:76:54:32:10
        Name: mi6test
`

func TestParseCHAPText_PicksNewestComplete(t *testing.T) {
	user, hash, err := ParseCHAPText(realisticCHAP)
	if err != nil {
		t.Fatalf("should parse: %v", err)
	}
	if user != "mi6test" {
		t.Fatalf("want newer username mi6test, got %q", user)
	}
	want := "fedcba9876543210fedcba9876543210:0123456789abcdef0123456789abcdef:05"
	if hash != want {
		t.Fatalf("want hash %s, got %s", want, hash)
	}
}

func TestParseCHAPText_IncompleteHandshake(t *testing.T) {
	// Challenge without any response must not yield a hash.
	out := "PPP Challenge Handshake Authentication Protocol\n" +
		"    Identifier: 0x07 (7)\n" +
		"    Challenge Value: 01:23:45:67:89:ab:cd:ef:01:23:45:67:89:ab:cd:ef\n"
	if _, _, err := ParseCHAPText(out); err == nil {
		t.Fatal("incomplete handshake must error")
	}
}

// realisticPAP mimics `tshark -V` output for a PAP Authenticate-Request.
const realisticPAP = `Point-to-Point Protocol
    Password Authentication Protocol
        Code: Authenticate-Request (1)
        Identifier: 0x01 (1)
        Length: 18
        Peer-ID Length: 4
        Peer-ID: mi6t
        Password Length: 6
        Password: cmwap
`

func TestParsePAPText(t *testing.T) {
	cred, err := ParsePAPText(realisticPAP)
	if err != nil {
		t.Fatalf("should parse: %v", err)
	}
	if cred.Username != "mi6t" || cred.Password != "cmwap" {
		t.Fatalf("wrong credential: %+v", cred)
	}
}

func TestParsePAPText_Missing(t *testing.T) {
	if _, err := ParsePAPText("nothing here\n"); err == nil {
		t.Fatal("expected error")
	}
	// Password without peer id is not a complete credential.
	half := "Password Authentication Protocol\n    Password: cmwap\n"
	if _, err := ParsePAPText(half); err == nil {
		t.Fatal("half credential must error")
	}
}

func TestValidateAuditConsent(t *testing.T) {
	if f, _ := ValidateAuditConsent(AuditConsent{}); f != "" {
		t.Fatalf("empty consent must pass for legacy calls, got %q", f)
	}
	if f, _ := ValidateAuditConsent(AuditConsent{IMSI: "001010123456789", ConfirmOwnership: true}); f != "" {
		t.Fatalf("owned consent rejected: %q", f)
	}
	for _, tc := range []AuditConsent{
		{IMSI: "001", ConfirmOwnership: true},
		{IMSI: "001010123456789"},
		{ConfirmOwnership: true},
	} {
		if f, _ := ValidateAuditConsent(tc); f == "" {
			t.Fatalf("bad consent accepted: %+v", tc)
		}
	}
}

func TestCheckWordlist(t *testing.T) {
	cfg := observationConfig(t, nil)
	cfg.DataDir = cfg.LogDir
	if err := CheckWordlist(cfg); err == nil {
		t.Fatal("missing wordlist must fail closed")
	}
	wordlistPath := cfg.WordlistPath()
	if err := os.WriteFile(wordlistPath, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckWordlist(cfg); err == nil {
		t.Fatal("empty wordlist must fail closed")
	}
	if err := os.WriteFile(wordlistPath, []byte("password\n123456\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckWordlist(cfg); err != nil {
		t.Fatalf("valid wordlist rejected: %v", err)
	}
}

func TestAuditLimitationsMentionsOwnership(t *testing.T) {
	unverified := AuditLimitations(false)
	verified := AuditLimitations(true)
	joined := func(ss []string) string {
		out := ""
		for _, s := range ss {
			out += s + "\n"
		}
		return out
	}
	if !strings.Contains(joined(unverified), "ownership not verified") {
		t.Fatalf("unverified limitations must urge consent: %v", unverified)
	}
	if !strings.Contains(joined(verified), "ownership not verified") {
		t.Fatalf("membership must not imply credential ownership: %v", verified)
	}
	for _, ss := range [][]string{unverified, verified} {
		j := joined(ss)
		if !strings.Contains(j, "CHAP identifier reuse") || strings.Contains(j, "never a mixed handshake") {
			t.Fatalf("must explain correlation limits without claiming snapshot isolation: %v", ss)
		}
		if !strings.Contains(j, "EPC log") || !strings.Contains(j, "hashcat -m 4800") {
			t.Fatalf("limitations must state EPC-vs-pcap and PAP/CHAP split: %v", ss)
		}
	}
}

func TestOpenAppendTightensExistingLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits are not implemented by Windows chmod")
	}
	path := filepath.Join(t.TempDir(), "audit.log")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := openAppend(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("log permissions: %o", st.Mode().Perm())
	}
	if _, err := f.WriteString("after\n"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "before\nafter\n" {
		t.Fatalf("append lost content: %q", content)
	}
}
