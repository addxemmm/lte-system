// Package parser extracts UE info from srsRAN_4G EPC logs.
// Keywords are stable from srsLTE -> srsRAN_4G (nas.cc / spgw):
// "ESM Info: APN", "Found User"/"Found UE context", "get_new_ue_ipv4 pool ip addr".
package parser

import (
	"bufio"
	"net/netip"
	"os"
	"strings"
)

// UEInfo is the first-attached UE summary (legacy only reported the first UE).
type UEInfo struct {
	APN   string
	IMSI  string
	IP    string
	Found bool
}

// ParseEPCLog scans the whole file and returns the LAST occurrence of each
// field (log appends; last line = most recent attach). Empty string = not seen.
func ParseEPCLog(path string) (UEInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return UEInfo{}, err
	}
	defer f.Close()
	var info UEInfo
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, "ESM Info: APN") {
			if v := lastField(line); v != "" {
				info.APN = v
				info.Found = true
			}
		}
		if strings.Contains(line, "Found User") || strings.Contains(line, "Found UE context") || strings.Contains(line, "Found previously") {
			if v := lastField(line); v != "" && looksLikeIMSI(v) {
				info.IMSI = v
				info.Found = true
			}
		}
		if strings.Contains(line, "pool ip addr") || strings.Contains(line, "static ip addr") || strings.Contains(line, "init_ue_ip") {
			if v := lastIPToken(line); v != "" {
				info.IP = v
				info.Found = true
			}
		}
	}
	return info, sc.Err()
}

func lastField(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[len(fields)-1], ".,;:\"'")
}

func looksLikeIMSI(s string) bool {
	if len(s) < 14 || len(s) > 16 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// lastIPToken finds the last dotted-quad in the line.
func lastIPToken(line string) string {
	fields := strings.Fields(line)
	for i := len(fields) - 1; i >= 0; i-- {
		tok := strings.Trim(fields[i], ".,;:\"'")
		if isIPv4(tok) {
			return tok
		}
	}
	return ""
}

func isIPv4(s string) bool {
	ip, err := netip.ParseAddr(s)
	return err == nil && ip.Is4()
}
