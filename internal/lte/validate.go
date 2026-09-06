package lte

import (
	"fmt"
	"strings"
)

// FieldIssue is one rejected field, serialized into 422 responses.
type FieldIssue struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// validNPRB is the whitelist srsenb accepts (6..100 PRB steps).
var validNPRB = map[int]bool{6: true, 15: true, 25: true, 50: true, 75: true, 100: true}

// ValidateDetailed strictly validates /start params for /api/v1.
// Unlike Validate (frozen legacy semantics: silent band fallback, no range
// checks), every problem is reported per-field and unknown bands are an
// error, never a silent fallback.
func (p StartParams) ValidateDetailed() []FieldIssue {
	var out []FieldIssue
	add := func(field, reason string) {
		out = append(out, FieldIssue{Field: field, Reason: reason})
	}
	if strings.TrimSpace(p.Band) == "" {
		add("band", "required")
	} else if _, ok := Table[p.Band]; !ok {
		add("band", fmt.Sprintf("must be one of %s", strings.Join(SupportedBands, ",")))
	}
	if strings.TrimSpace(p.APN) == "" {
		add("apn", "required")
	} else if strings.ContainsAny(p.APN, " \t\n\r\"';&|<>$`\\") {
		add("apn", "contains illegal characters")
	}
	if len(p.MCC) != 3 || !isDigits(p.MCC) {
		add("mcc", "must be 3 digits")
	}
	if !(len(p.MNC) == 2 || len(p.MNC) == 3) || !isDigits(p.MNC) {
		add("mnc", "must be 2 or 3 digits")
	}
	if strings.TrimSpace(p.Network) == "" {
		add("network", "required")
	} else if strings.ContainsAny(p.Network, " \t\n\r\"';&|<>$`\\") {
		add("network", "contains illegal characters")
	}
	switch p.SDR {
	case "", "auto", "uhd", "bladerf", "zmq":
	default:
		add("sdr", "must be one of auto,uhd,bladerf,zmq")
	}
	if p.TxGain != nil && (*p.TxGain < 0 || *p.TxGain > 90) {
		add("tx_gain", "must be 0..90")
	}
	if p.RxGain != nil && (*p.RxGain < 0 || *p.RxGain > 90) {
		add("rx_gain", "must be 0..90")
	}
	if p.NPRB != nil && !validNPRB[*p.NPRB] {
		add("n_prb", "must be one of 6,15,25,50,75,100")
	}
	if err := validateNetName(p.FullNetName); err != nil {
		add("full_net_name", err.Error())
	}
	if err := validateNetName(p.ShortNetName); err != nil {
		add("short_net_name", err.Error())
	}
	if p.DNS != "" && !validIPv4(p.DNS) {
		add("dns", "must be an IPv4 address")
	}
	return out
}

// IsTDD reports whether band uses TDD (UL EARFCN == DL EARFCN).
func (b BandInfo) IsTDD() bool { return b.ULMHz == 0 }
