package crack

import "testing"

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
