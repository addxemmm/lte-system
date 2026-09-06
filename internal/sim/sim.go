// Package sim implements flexible SIM/USIM programming.
//
// Legacy hard-coded everything: Ki=001122.., OPc=63bfa5.., ADM=3030..,
// ICCID=89860123456789012345, card type "testsim", and IMSI-only request.
// This package keeps {imsi}-only requests working but allows every key
// parameter to be overridden per-request, with server-side defaults from config.
package sim

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/addxemmm/lte-system/internal/config"
)

// WriteRequest is POST /writesim body. All fields except IMSI are optional.
type WriteRequest struct {
	IMSI  string `json:"imsi"`
	MCC   string `json:"mcc"`   // default: imsi[0:3]
	MNC   string `json:"mnc"`   // default: imsi[3:5] (2-digit); use MNC3 for 3-digit
	MNC3  bool   `json:"mnc3"`  // set true when MNC is 3 digits
	ICCID string `json:"iccid"` // default server iccid; "auto" = don't pass -s (keep factory)
	Ki    string `json:"ki"`    // 32 hex, default server ki
	OP    string `json:"op"`    // 32 hex, mutually exclusive with OPc
	OPc   string `json:"opc"`   // 32 hex, default server opc
	OPType string `json:"op_type"` // "op" or "opc", default server value
	Auth  string `json:"auth"`  // "mil" | "xor", default "mil"
	AMF   string `json:"amf"`   // 4 hex, default "8001" (matches example card row)
	ACC   string `json:"acc"`   // 4 hex Access Control Class, default "FFFF"
	ADM   string `json:"adm"`   // hex string, default "3030303030303030"
	SPN   string `json:"spn"`   // operator name, default "LTESystem"
	Name  string `json:"name"`  // user_db.csv Name column, default auto ueN
	SQN   string `json:"sqn"`   // 12 hex sequence, default random
	QCI   *int   `json:"qci"`
	Card  string `json:"card"` // pysim card type, default "testsim"
	PinADM string `json:"pin_adm"` // rarely needed; ADM hex is used as -A
}

// Resolved is a validated request with defaults filled.
type Resolved struct {
	WriteRequest
	OPValue string // effective 32-hex OP/OPc
	UseOPc  bool
}

var (
	reHex32 = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
	reHex4  = regexp.MustCompile(`^[0-9a-fA-F]{4}$`)
	reHex12 = regexp.MustCompile(`^[0-9a-fA-F]{12}$`)
)

// Resolve validates req and fills defaults from cfg.
func Resolve(req WriteRequest, cfg config.Config) (Resolved, error) {
	r := Resolved{WriteRequest: req}
	d := cfg.Sim
	if !regexp.MustCompile(`^\d{15}$`).MatchString(req.IMSI) {
		return r, fmt.Errorf("imsi must be 15 digits")
	}
	if r.MCC == "" {
		r.MCC = req.IMSI[:3]
	}
	if r.MNC == "" {
		if r.MNC3 {
			r.MNC = req.IMSI[3:6]
		} else {
			r.MNC = req.IMSI[3:5]
		}
	}
	if r.ICCID == "" {
		r.ICCID = d.ICCID
	}
	if r.Ki == "" {
		r.Ki = d.Ki
	}
	if !reHex32.MatchString(r.Ki) {
		return r, fmt.Errorf("ki must be 32 hex chars")
	}
	// OP / OPc resolution
	op, opc := strings.TrimSpace(r.OP), strings.TrimSpace(r.OPc)
	if op != "" && opc != "" {
		return r, fmt.Errorf("op and opc are mutually exclusive")
	}
	if op == "" && opc == "" {
		if d.OP != "" {
			op = d.OP
		} else {
			opc = d.OPc
		}
	}
	if op != "" {
		if !reHex32.MatchString(op) {
			return r, fmt.Errorf("op must be 32 hex chars")
		}
		r.OPValue = strings.ToLower(op)
		r.UseOPc = false
		if r.OPType == "" {
			r.OPType = "op"
		}
	} else {
		if !reHex32.MatchString(opc) {
			return r, fmt.Errorf("opc must be 32 hex chars")
		}
		r.OPValue = strings.ToLower(opc)
		r.UseOPc = true
		if r.OPType == "" {
			r.OPType = "opc"
		}
	}
	if r.Auth == "" {
		r.Auth = d.Auth
	}
	if r.Auth != "mil" && r.Auth != "xor" {
		return r, fmt.Errorf("auth must be mil or xor")
	}
	if r.AMF == "" {
		r.AMF = d.AMF
	}
	if !reHex4.MatchString(r.AMF) {
		return r, fmt.Errorf("amf must be 4 hex chars")
	}
	if r.ACC == "" {
		r.ACC = d.ACC
	}
	if !reHex4.MatchString(r.ACC) {
		return r, fmt.Errorf("acc must be 4 hex chars")
	}
	if r.ADM == "" {
		r.ADM = d.ADM
	}
	if r.SPN == "" {
		r.SPN = d.SPN
	}
	if r.Card == "" {
		r.Card = d.Card
	}
	if r.SQN == "" {
		var b [6]byte
		if _, err := rand.Read(b[:]); err != nil {
			r.SQN = "000000001234"
		} else {
			r.SQN = hex.EncodeToString(b[:])
		}
	}
	if !reHex12.MatchString(r.SQN) {
		return r, fmt.Errorf("sqn must be 12 hex chars")
	}
	if r.QCI == nil {
		q := d.QCI
		if q == 0 {
			q = 7
		}
		r.QCI = &q
	}
	return r, nil
}

// ProgArgs builds the pySim-prog.py argv (no shell). Mirrors legacy:
// pySim-prog.py -p 0 -x MCC -y MNC -i IMSI [-s ICCID] -o OPc|-k Ki ... -n SPN -A ADM --acc ACC -t CARD
func (r Resolved) ProgArgs(pysimDir string) []string {
	prog := filepath.Join(pysimDir, "pySim-prog.py")
	args := []string{prog, "-p", "0", "-x", r.MCC, "-y", r.MNC, "-i", r.IMSI}
	if r.ICCID != "" && !strings.EqualFold(r.ICCID, "auto") {
		args = append(args, "-s", r.ICCID)
	}
	// Legacy passed both -o and -k; pysim testsim honors IMSI/PLMN files while Ki/OPc
	// program the auth files on cards that support it (SJS1/SJA2). Keep both.
	if r.UseOPc {
		args = append(args, "-o", r.OPValue)
	} else {
		args = append(args, "--op", r.OPValue)
	}
	args = append(args, "-k", strings.ToLower(r.Ki))
	args = append(args, "-n", r.SPN, "-A", r.ADM, "--acc", r.ACC, "-t", r.Card)
	return args
}

// Program runs the full write -> verify -> user_db.csv flow.
func Program(ctx context.Context, cfg config.Config, req WriteRequest) (msgID int, msg string, err error) {
	r, err := Resolve(req, cfg)
	if err != nil {
		return 6, "Invalid parameters: " + err.Error(), err
	}
	// 1. pcscd + reader/card presence
	if err := restartPCSCD(ctx); err != nil {
		// non-fatal; reader may already be up
		_ = err
	}
	present, hasCard := checkReader(ctx, cfg)
	if !present {
		return 2, "Device is not connected, please connect acr1281 first.", fmt.Errorf("no reader")
	}
	if !hasCard {
		return 5, "SIM card is not inserted.", fmt.Errorf("no card")
	}
	// 2. program (run inside PySimDir so bundled imports resolve)
	ctx2, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	argv := r.ProgArgs(cfg.PySimDir)
	progCmd := exec.CommandContext(ctx2, "python3", argv...)
	progCmd.Dir = cfg.PySimDir
	out, err := progCmd.CombinedOutput()
	sout := string(out)
	if err != nil || !strings.Contains(sout, "Programming successful") {
		return 0, "Failed.", fmt.Errorf("program failed: %v %.500s", err, sout)
	}
	// 3. read-back verify
	ctx3, cancel3 := context.WithTimeout(ctx, 30*time.Second)
	defer cancel3()
	readCmd := exec.CommandContext(ctx3, "python3",
		filepath.Join(cfg.PySimDir, "pySim-read.py"), "-p", "0")
	readCmd.Dir = cfg.PySimDir
	readOut, err := readCmd.CombinedOutput()
	if err != nil || !strings.Contains(string(readOut), r.IMSI) {
		return 0, "Failed.", fmt.Errorf("verify failed: %v %.500s", err, string(readOut))
	}
	// 4. append to user_db.csv
	st, err := AddUser(cfg.UserDBPath(), r)
	if err != nil {
		return 3, "Writting card successfully, but write user_db.csv failed.", err
	}
	if st == "exists" {
		return 4, "The card already exists and can be used directly.", nil
	}
	return 1, "Succeed.", nil
}

func restartPCSCD(ctx context.Context) error {
	ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx2, "service", "pcscd", "restart").Run()
}

func checkReader(ctx context.Context, cfg config.Config) (present, hasCard bool) {
	// Layer 1: USB level (fast, no side effects). Matches ACR128* readers
	// and any Advanced Card Systems device (VID 072f: ACR122U/ACR125x/...).
	ctx1, cancel1 := context.WithTimeout(ctx, 8*time.Second)
	defer cancel1()
	if out, err := exec.CommandContext(ctx1, "lsusb").CombinedOutput(); err == nil {
		if readerPresentLsusb(string(out)) {
			return true, probeCard(ctx, cfg)
		}
	}
	// Layer 2: PCSC level (authoritative). pcsc_scan -n blocks while no
	// reader exists, hence the hard timeout; spinner output without any
	// "Reader" line conclusively means no reader.
	ctx2, cancel2 := context.WithTimeout(ctx, 8*time.Second)
	defer cancel2()
	out, _ := exec.CommandContext(ctx2, "pcsc_scan", "-n").CombinedOutput()
	s := string(out)
	if present, card := parsePcscScan(s); present {
		return present, card
	}
	if strings.Contains(s, "Waiting for the first reader") ||
		strings.Contains(s, "Scanning present readers") {
		return false, false
	}
	// Layer 3: last resort for broken lsusb/pcsc stacks — direct pySim
	// probe with strict semantics (any error counts as no reader, so a
	// Python traceback can never again masquerade as "card missing").
	return probeCardStrict(ctx, cfg)
}

// readerPresentLsusb reports an ACS smartcard reader in lsusb output.
func readerPresentLsusb(out string) bool {
	up := strings.ToUpper(out)
	return strings.Contains(up, "ACR128") || strings.Contains(up, "072F")
}

// parsePcscScan interprets `pcsc_scan -n` output: present when any
// "Reader N: ..." line exists; card when a "Card inserted" state shows.
func parsePcscScan(out string) (present, card bool) {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Reader ") {
			present = true
		}
	}
	if strings.Contains(out, "Card inserted") {
		card = true
	}
	return present, card
}

// cardEvidence is true for a genuine successful card read: expected
// keywords present and no Python traceback (tracebacks must never count
// as evidence — that bug once reported every reader-less host as
// "SIM card is not inserted").
func cardEvidence(out string) bool {
	if strings.Contains(out, "Traceback") {
		return false
	}
	return strings.Contains(out, "Reading") ||
		strings.Contains(out, "ATR") ||
		strings.Contains(out, "IMSI")
}

func pySimRead(ctx context.Context, cfg config.Config, timeout time.Duration) (string, error) {
	c, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(c, "python3",
		filepath.Join(cfg.PySimDir, "pySim-read.py"), "-p", "0")
	cmd.Dir = cfg.PySimDir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// probeCard checks for a card when a reader is already established
// (USB or PCSC level). Lenient on exit code (some readers exit nonzero
// on warnings) but strict on evidence content.
func probeCard(ctx context.Context, cfg config.Config) bool {
	out, _ := pySimRead(ctx, cfg, 15*time.Second)
	return cardEvidence(out)
}

// probeCardStrict is the last-resort probe with no other evidence:
// requires a clean exit AND read evidence.
func probeCardStrict(ctx context.Context, cfg config.Config) (bool, bool) {
	out, err := pySimRead(ctx, cfg, 15*time.Second)
	if err != nil || !cardEvidence(out) {
		return false, false
	}
	return true, true
}

// AddUser appends "Name,Auth,IMSI,Key,OP_Type,OP,AMF,SQN,QCI,IP_alloc" to user_db.csv.
// Returns "added" or "exists". It parses CSV properly (legacy used magic offset 1738).
func AddUser(userDBPath string, r Resolved) (string, error) {
	var existing []byte
	if b, err := os.ReadFile(userDBPath); err == nil {
		existing = b
	}
	if strings.Contains(string(existing), r.IMSI) {
		return "exists", nil
	}
	name := r.Name
	if name == "" {
		name = nextUEName(string(existing))
	}
	qci := 7
	if r.QCI != nil {
		qci = *r.QCI
	}
	line := fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s,%d,dynamic\n",
		name, r.Auth, r.IMSI, strings.ToLower(r.Ki), r.OPType, r.OPValue,
		strings.ToUpper(r.AMF), strings.ToLower(r.SQN), qci)
	f, err := os.OpenFile(userDBPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		return "", err
	}
	return "added", nil
}

func nextUEName(content string) string {
	max := -1
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, ",")
		if len(cols) == 0 {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(cols[0], "ue%d", &n); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("ue%d", max+1)
}

// UEEntry is one HSS row summary for /profile (no secrets: key material omitted).
type UEEntry struct {
	Name string `json:"name"`
	Auth string `json:"auth"`
	IMSI string `json:"imsi"`
}

// Summarize parses user_db.csv into UE entries. Missing file => empty list, nil error.
func Summarize(userDBPath string) ([]UEEntry, error) {
	b, err := os.ReadFile(userDBPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []UEEntry{}, nil
		}
		return nil, err
	}
	out := []UEEntry{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, ",")
		if len(cols) < 3 {
			continue
		}
		out = append(out, UEEntry{
			Name: strings.TrimSpace(cols[0]),
			Auth: strings.TrimSpace(cols[1]),
			IMSI: strings.TrimSpace(cols[2]),
		})
	}
	return out, nil
}
