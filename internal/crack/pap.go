// PAP credential extraction.
//
// When the UE uses PAP (instead of CHAP) for APN authentication, the
// username and password travel in cleartext inside the S1AP capture, so no
// hashcat run is needed: extraction IS the audit verdict.
//
// Wire format (Wireshark "Password Authentication Protocol" tree) looks like:
//
//	Password Authentication Protocol
//	    Code: Authenticate-Request (1)
//	    Identifier: 0x01 (1)
//	    Length: 18
//	    Peer-ID Length: 4
//	    Peer-ID: mi6t
//	    Password Length: 6
//	    Password: cmwap
//
// Field labels vary slightly across tshark versions, so the parser matches
// case-insensitive keys and takes the value after the first colon.
package crack

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/addxemmm/lte-system/internal/config"
)

// PAPCredential is a plaintext APN credential pair.
type PAPCredential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ExtractPAP runs tshark over the S1AP capture and returns the first
// complete PAP Authenticate-Request credential pair.
//
// Like ExtractCHAP it inspects a bounded private immutable complete-record
// prefix so a concurrently appended tail cannot poison the verdict.
func ExtractPAP(ctx context.Context, cfg config.Config) (PAPCredential, error) {
	var zero PAPCredential
	snapshotPath, cleanup, err := snapshotForExtraction(ctx, cfg)
	if err != nil {
		return zero, err
	}
	defer cleanup()
	return ExtractPAPFromFile(ctx, cfg, snapshotPath)
}

// ExtractPAPFromFile runs tshark over one immutable pcap path.
func ExtractPAPFromFile(ctx context.Context, cfg config.Config, pcapPath string) (PAPCredential, error) {
	var zero PAPCredential
	args := []string{
		"-o", `uat:user_dlts:"User 3 (DLT=150)","s1ap","0","","0",""`,
		"-r", pcapPath, "-Y", "pap", "-V",
	}
	ctx2, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx2, cfg.TsharkBin, args...).CombinedOutput()
	if err != nil && len(out) == 0 {
		return zero, fmt.Errorf("tshark: %w", err)
	}
	return ParsePAPText(string(out))
}

// ParsePAPText is the pure, unit-testable part: scan `tshark -V` output for
// the first Authenticate-Request block carrying both a peer id and a password.
func ParsePAPText(tsharkOut string) (PAPCredential, error) {
	var zero PAPCredential
	// Split into per-packet-ish blocks on the protocol header; each block is
	// scanned independently so a truncated tail cannot poison an earlier one.
	blocks := strings.Split(tsharkOut, "Password Authentication Protocol")
	for _, b := range blocks[1:] {
		cred, ok := parsePAPBlock(b)
		if ok {
			return cred, nil
		}
	}
	return zero, fmt.Errorf("no PAP credentials found")
}

func parsePAPBlock(block string) (PAPCredential, bool) {
	var cred PAPCredential
	for _, line := range strings.Split(block, "\n") {
		l := strings.TrimSpace(line)
		low := strings.ToLower(l)
		// Value lives after the first colon: "Peer-ID: mi6t".
		colon := strings.Index(l, ":")
		if colon < 0 {
			continue
		}
		key, val := strings.TrimSpace(low[:colon]), strings.TrimSpace(l[colon+1:])
		val = strings.Trim(val, "\"'")
		if val == "" {
			continue
		}
		switch key {
		case "peer-id", "peer id", "username", "user-name":
			if cred.Username == "" {
				cred.Username = val
			}
		case "password", "passwd":
			if cred.Password == "" {
				cred.Password = val
			}
		}
		if cred.Username != "" && cred.Password != "" {
			return cred, true
		}
	}
	return PAPCredential{}, false
}
