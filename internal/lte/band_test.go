package lte

import "testing"

func TestLookup_Known(t *testing.T) {
	b, ok := Lookup("3")
	if !ok || b.DLEARFCN != 1575 {
		t.Fatalf("band 3 wrong: %+v %v", b, ok)
	}
	b, ok = Lookup("41")
	if !ok || b.DLEARFCN != 40620 {
		t.Fatalf("band 41 wrong: %+v", b)
	}
}

// TestULEARFCN pins the explicit UL EARFCN per band (3GPP TS 36.101:
// FDD UL = DL+18000, TDD UL = DL). srsRAN_4G's internal derivation fails
// for TDD, so these values go verbatim into rr.conf.
func TestULEARFCN(t *testing.T) {
	want := map[string]int{
		"1": 18300, "3": 19575, "5": 20525, "7": 21350, "8": 21625,
		"34": 36275, "39": 38450, "40": 39150, "41": 40620,
	}
	for band, ul := range want {
		b, ok := Lookup(band)
		if !ok {
			t.Fatalf("band %s unknown", band)
		}
		if b.ULEARFCN != ul {
			t.Fatalf("band %s ul_earfcn = %d, want %d", band, b.ULEARFCN, ul)
		}
		if b.ULMHz == 0 && b.ULEARFCN != b.DLEARFCN {
			t.Fatalf("TDD band %s must have ul==dl earfcn", band)
		}
	}
}

func TestLookup_UnknownFallsBack(t *testing.T) {
	b, ok := Lookup("99")
	if ok {
		t.Fatalf("expected unknown, got %v", b)
	}
	if b.DLEARFCN != 3350 {
		t.Fatalf("fallback earfcn must be 3350 (legacy default), got %d", b.DLEARFCN)
	}
	if len(SupportedBands) != 9 {
		t.Fatalf("supported bands changed: %v", SupportedBands)
	}
}
