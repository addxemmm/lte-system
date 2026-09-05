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
