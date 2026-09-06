// Package lte holds the EARFCN/band table and srsRAN_4G config rendering.
// Legacy run.sh mapping is preserved verbatim (unknown band -> default 7/41 handled by caller).
package lte

// BandInfo maps a user-facing band number to DL/UL EARFCN + display frequencies.
type BandInfo struct {
	Band     string
	DLEARFCN int
	ULEARFCN int // explicit UL EARFCN for rr.conf. FDD = DL+18000, TDD = DL.
	DLMHz    float64
	ULMHz    float64 // 0 means TDD / no uplink freq display
}

// Table preserves the v1.x run.sh case mapping, plus UL EARFCN.
// UL derivation inside srsRAN_4G fails for TDD bands, so we always pass
// ul_earfcn explicitly (verified against 3GPP TS 36.101 earfcn tables).
var Table = map[string]BandInfo{
	"1":  {Band: "1", DLEARFCN: 300, ULEARFCN: 18300, DLMHz: 2140, ULMHz: 1950},
	"3":  {Band: "3", DLEARFCN: 1575, ULEARFCN: 19575, DLMHz: 1842.5, ULMHz: 1747.5},
	"5":  {Band: "5", DLEARFCN: 2525, ULEARFCN: 20525, DLMHz: 881.5, ULMHz: 836.5},
	"7":  {Band: "7", DLEARFCN: 3350, ULEARFCN: 21350, DLMHz: 2680, ULMHz: 2560},
	"8":  {Band: "8", DLEARFCN: 3625, ULEARFCN: 21625, DLMHz: 942.5, ULMHz: 897.5},
	"34": {Band: "34", DLEARFCN: 36275, ULEARFCN: 36275, DLMHz: 2017.5, ULMHz: 0},
	"39": {Band: "39", DLEARFCN: 38450, ULEARFCN: 38450, DLMHz: 1900, ULMHz: 0},
	"40": {Band: "40", DLEARFCN: 39150, ULEARFCN: 39150, DLMHz: 2350, ULMHz: 0},
	"41": {Band: "41", DLEARFCN: 40620, ULEARFCN: 40620, DLMHz: 2593, ULMHz: 0},
}

// SupportedBands lists valid band strings for API validation/docs.
var SupportedBands = []string{"1", "3", "5", "7", "8", "34", "39", "40", "41"}

// Lookup returns BandInfo + true if known. Unknown bands return (default, false)
// so the caller can fall back and document it (legacy fell back to 3350 silently).
func Lookup(band string) (BandInfo, bool) {
	if b, ok := Table[band]; ok {
		return b, true
	}
	return BandInfo{Band: band, DLEARFCN: 3350, ULEARFCN: 21350, DLMHz: 2680, ULMHz: 2560}, false
}
