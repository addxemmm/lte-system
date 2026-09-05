package crack

import "testing"

func TestIsHashFormat(t *testing.T) {
	if !isHashFormat("aabbcc:112233:01") {
		t.Fatal("valid rejected")
	}
	if isHashFormat("zzz:11:01") || isHashFormat("a:b") || isHashFormat("") {
		t.Fatal("invalid accepted")
	}
}

func TestParseCHAPText_Missing(t *testing.T) {
	if _, _, err := ParseCHAPText("nothing here\n"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeHexByte(t *testing.T) {
	if normalizeHexByte("1") != "01" || normalizeHexByte("A") != "0a" {
		t.Fatal("pad failed")
	}
}
