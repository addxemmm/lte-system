// Package lte manages the srsRAN_4G EPC+eNB lifecycle (stateless tool side).
package lte

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/addxemmm/lte-system/internal/config"
	"github.com/addxemmm/lte-system/internal/sdr"
	"github.com/addxemmm/lte-system/internal/subscriber"
	"github.com/addxemmm/lte-system/internal/sysop"
)

// StartParams mirrors POST /start (legacy fields + SDR extensions).
type StartParams struct {
	Band              string `json:"band"`
	APN               string `json:"apn"`
	APNMismatchPolicy string `json:"apn_mismatch_policy"`
	MCC               string `json:"mcc"`
	MNC               string `json:"mnc"`
	Network           string `json:"network"`
	SDR               string `json:"sdr"`         // "uhd"|"bladerf"|"zmq"|"auto"|""
	DeviceArgs        string `json:"device_args"` // optional UHD/bladeRF args
	TxGain            *int   `json:"tx_gain"`
	RxGain            *int   `json:"rx_gain"`
	NPRB              *int   `json:"n_prb"`
	// Operator name shown on the UE (NITZ via EMM Information).
	// Empty = server default (legacy display "srsRAN").
	FullNetName  string `json:"full_net_name"`
	ShortNetName string `json:"short_net_name"`
	// DNS server handed to UEs via PCO. Empty = server default.
	DNS      string `json:"dns"`
	UESubnet string `json:"ue_subnet"` // canonical RFC1918 /24; empty = 172.16.0.0/24
	UEAccess string `json:"ue_access"` // "isolated" (default) or "allow"
}

// Validate checks legacy "incomplete parameters" + new field formats.
func (p StartParams) Validate() error {
	if strings.TrimSpace(p.Band) == "" || strings.TrimSpace(p.APN) == "" ||
		strings.TrimSpace(p.MCC) == "" || strings.TrimSpace(p.MNC) == "" {
		return fmt.Errorf("incomplete parameters")
	}
	if len(p.MCC) != 3 || !isDigits(p.MCC) {
		return fmt.Errorf("mcc must be 3 digits")
	}
	if !(len(p.MNC) == 2 || len(p.MNC) == 3) || !isDigits(p.MNC) {
		return fmt.Errorf("mnc must be 2 or 3 digits")
	}
	if err := validateAPN(p.APN); err != nil {
		return fmt.Errorf("apn %w", err)
	}
	if err := validateAPNMismatchPolicy(p.APNMismatchPolicy); err != nil {
		return fmt.Errorf("apn_mismatch_policy %w", err)
	}
	network := strings.TrimSpace(p.Network)
	if network != "" && network != "auto" && !validInterfaceName(network) {
		return fmt.Errorf("network is not a valid interface name")
	}
	if err := validateNetName(p.FullNetName); err != nil {
		return fmt.Errorf("full_net_name: %w", err)
	}
	if err := validateNetName(p.ShortNetName); err != nil {
		return fmt.Errorf("short_net_name: %w", err)
	}
	if p.DNS != "" && !validIPv4(p.DNS) {
		return fmt.Errorf("dns must be an IPv4 address")
	}
	ueSubnet, ueAccess := strings.TrimSpace(p.UESubnet), strings.ToLower(strings.TrimSpace(p.UEAccess))
	if ueSubnet == "" {
		ueSubnet = defaultUESubnet
	}
	if ueAccess == "" {
		ueAccess = defaultUEAccess
	}
	if _, err := makeUENetworkPlan(ueSubnet, ueAccess); err != nil {
		return err
	}
	return nil
}

// validateNetName allows empty (use server default) or 1-32 printable
// ASCII chars safe for libconfig (no quotes/semicolons/shell metachars).
func validateNetName(s string) error {
	if s == "" {
		return nil
	}
	if len(s) > 32 {
		return fmt.Errorf("must be 1-32 chars")
	}
	for _, r := range s {
		if r < 0x20 || r > 0x7e || strings.ContainsRune("\"';#$`\\", r) {
			return fmt.Errorf("illegal character %q", r)
		}
	}
	return nil
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// checkIface verifies the uplink interface exists. Skipped where the
// sysfs network view is absent (non-Linux dev machines running unit tests).
func checkIface(name string) error {
	entries, err := os.ReadDir(netInterfaceDir)
	if err != nil {
		if runtime.GOOS != "linux" && os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect network interfaces: %w", err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("network interface list is empty")
	}
	for _, e := range entries {
		if e.Name() == name {
			return nil
		}
	}
	return fmt.Errorf("unknown network interface %q", name)
}

// validIPv4 is a strict dotted-quad check (no leading-zero octets > 255).
func validIPv4(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 3 {
			return false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
		}
		if len(p) > 1 && p[0] == '0' {
			return false
		}
		n := 0
		for _, r := range p {
			n = n*10 + int(r-'0')
		}
		if n > 255 {
			return false
		}
	}
	return true
}

// Manager owns EPC/eNB/tcpdump processes and rendered configs.
type Manager struct {
	cfg config.Config
	mu  sync.Mutex

	epcCmd  *managedChild
	enbCmd  *managedChild
	pcapCmd *managedChild

	startedAt time.Time
	lastStart StartParams
	// lastNetwork snapshots the resolved uplink iface of the current run so
	// Stop removes the right rules even when the persisted policy is "auto".
	// It is cleared on successful Stop and is never written into the profile.
	lastNetwork     string
	lastUESubnet    string
	lastUEAccess    string
	natOwned        bool
	forwardingOwned []iptRule
	runID           string
	ueSnapshotPath  string
	lastBand        BandInfo
	bandKnown       bool
}

// Startup grace periods are variables so lifecycle tests can exercise
// cancellation without sleeping for the production RF initialization window.
var (
	epcInitDelay = 3 * time.Second
	enbInitDelay = 5 * time.Second
)

// New creates a Manager.
func New(cfg config.Config) *Manager { return &Manager{cfg: cfg} }

// Status is the machine-readable state for /status and /healthz.
type Status struct {
	Running           bool       `json:"running"`
	EPC               bool       `json:"epc"`
	ENB               bool       `json:"enb"`
	Pcap              bool       `json:"pcap"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	Band              string     `json:"band,omitempty"`
	APN               string     `json:"apn,omitempty"`
	APNMismatchPolicy string     `json:"apn_mismatch_policy,omitempty"`
	NetName           string     `json:"net_name,omitempty"`
	Network           string     `json:"network,omitempty"`
	ResolvedNetwork   string     `json:"resolved_network,omitempty"`
}

// IsRunning reports live state. Only non-zombie, non-exited processes
// count: a srsenb that died on RF init (or any defunct child) must read
// as stopped, never as running.
func (m *Manager) IsRunning() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	epc := alive(m.epcCmd)
	enb := alive(m.enbCmd)
	pcap := alive(m.pcapCmd)
	st := Status{EPC: epc, ENB: enb, Pcap: pcap, Running: epc || enb}
	if st.Running && !m.startedAt.IsZero() {
		t := m.startedAt
		st.StartedAt = &t
		st.Band = m.lastStart.Band
		st.APN = m.lastStart.APN
		st.APNMismatchPolicy = effectiveAPNMismatchPolicy(m.lastStart.APNMismatchPolicy)
		st.NetName = m.lastStart.FullNetName
		st.Network = m.lastStart.Network
		st.ResolvedNetwork = m.lastNetwork
	}
	return st
}

// Start renders configs and launches iptables -> srsepc -> srsenb -> tcpdump.
// It mirrors legacy run.sh but fixes the hard-coded srsenb path and adds SDR choice.
func (m *Manager) Start(ctx context.Context, p StartParams) (bandKnown bool, err error) {
	p.Network = strings.TrimSpace(p.Network)
	if p.Network == "" {
		p.Network = "auto"
	}
	normalizeAPNMismatchPolicy(&p)
	normalizeUEPolicy(&p)
	if err := p.Validate(); err != nil {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.anyAliveLocked() || sysop.Running("srsepc") || sysop.Running("srsenb") {
		return false, fmt.Errorf("is running")
	}
	// Clear handles/rules left by a run whose children all exited naturally.
	// Foreign same-name processes are never adopted or stopped.
	if m.hasLifecycleLocked() {
		if !m.stopLocked() {
			return false, fmt.Errorf("previous owned processes or network rules could not be cleaned up")
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	resolvedNetwork, err := resolveNetwork(p.Network)
	if err != nil {
		return false, fmt.Errorf("resolve uplink network %q: %w", p.Network, err)
	}
	if err := requireIPv4Forwarding(); err != nil {
		return false, fmt.Errorf("network preflight: %w", err)
	}
	if err := m.cfg.EnsureDirs(); err != nil {
		return false, err
	}
	uePlan, err := makeUENetworkPlan(p.UESubnet, p.UEAccess)
	if err != nil {
		return false, err
	}
	if err := m.validateUENetworkEnvironment(uePlan); err != nil {
		return false, fmt.Errorf("UE network preflight: %w", err)
	}

	band, known := Lookup(p.Band)
	det := sdr.Detect()
	// Legacy required B210; now allow bladeRF/ZMQ too so the tool works without USRP.
	if p.SDR == "" || p.SDR == "auto" {
		if !det.UHD_B210 && !det.BladeRF {
			// Still allow zmq/test start if explicitly requested; otherwise report no device.
			if p.SDR != "zmq" {
				return known, fmt.Errorf("device is not connected, please connect usrp device")
			}
		}
	} else if p.SDR == "uhd" && !det.UHD_B210 {
		return known, fmt.Errorf("device is not connected, please connect usrp device")
	}

	txGain, rxGain, nPRB := m.cfg.DefaultTxGain, m.cfg.DefaultRxGain, m.cfg.DefaultNRB
	if p.TxGain != nil {
		txGain = *p.TxGain
	} else {
		p.TxGain = &txGain
	}
	if p.RxGain != nil {
		rxGain = *p.RxGain
	} else {
		p.RxGain = &rxGain
	}
	if p.NPRB != nil {
		nPRB = *p.NPRB
	} else {
		p.NPRB = &nPRB
	}
	// Resolve display network names (request overrides server defaults).
	if p.FullNetName == "" {
		p.FullNetName = m.cfg.DefaultFullNetName
	}
	if p.ShortNetName == "" {
		p.ShortNetName = m.cfg.DefaultShortNetName
	}
	if p.DNS == "" {
		p.DNS = m.cfg.DefaultDNS
	}
	devName, devArgs := sdr.SelectArgs(p.SDR, p.DeviceArgs, det, m.cfg)

	if err := m.renderAll(p, band, devName, devArgs, txGain, rxGain, nPRB); err != nil {
		return known, err
	}
	if err := ctx.Err(); err != nil {
		return known, err
	}
	runID, err := generateRunID()
	if err != nil {
		return known, fmt.Errorf("generate UE telemetry run ID: %w", err)
	}
	ueSnapshotPath := m.cfg.LogPath("ue-sessions.json")
	if err := os.Remove(ueSnapshotPath); err != nil && !os.IsNotExist(err) {
		return known, fmt.Errorf("remove stale UE telemetry snapshot: %w", err)
	}

	// Network policy is established before either LTE process is started, so
	// permissions, xtables locking and forwarding failures cannot surface only
	// after RF transmission begins. Each helper reports exactly which rules it
	// inserted; rollback removes those rules and leaves pre-existing host rules.
	m.lastNetwork = resolvedNetwork
	m.lastUESubnet = uePlan.SubnetText
	m.lastUEAccess = uePlan.Access
	m.natOwned, err = ensureRule(ctx, natRule(resolvedNetwork, uePlan.SubnetText))
	if err != nil {
		m.stopLocked()
		return known, fmt.Errorf("configure UE NAT on %q: %w", resolvedNetwork, err)
	}
	m.forwardingOwned, err = ensureForwarding(ctx, resolvedNetwork, uePlan)
	if err != nil {
		m.stopLocked()
		return known, fmt.Errorf("configure UE forwarding on %q: %w", resolvedNetwork, err)
	}
	if err := ctx.Err(); err != nil {
		m.stopLocked()
		return known, err
	}

	epcConf := filepath.Join(m.cfg.ConfDir, "epc_run.conf")
	enbConf := filepath.Join(m.cfg.ConfDir, "enb_run.conf")
	epcRunLog := filepath.Join(m.cfg.LogDir, "epc_run.log")
	enbRunLog := filepath.Join(m.cfg.LogDir, "enb_run.log")

	// 1. srsepc
	epcLogF, err := os.Create(epcRunLog)
	if err != nil {
		m.stopLocked()
		return known, err
	}
	epcCmd := exec.CommandContext(context.Background(), m.cfg.SrsEPCBin, epcConf)
	epcCmd.Env = epcEnvironment(os.Environ(), ueSnapshotPath, runID, p.APNMismatchPolicy)
	epcCmd.Stdout = epcLogF
	epcCmd.Stderr = epcLogF
	epc, err := startManaged(epcCmd)
	// Start duplicates the descriptor into the child. The Manager must not
	// retain its parent copy for the lifetime of the cell.
	_ = epcLogF.Close()
	if err != nil {
		m.stopLocked()
		return known, fmt.Errorf("start epc: %w", err)
	}
	m.epcCmd = epc
	if err := waitForInit(ctx, epcInitDelay); err != nil {
		m.stopLocked()
		return known, err
	}
	if !alive(epc) {
		m.stopLocked()
		return known, fmt.Errorf("epc exited early, see %s", epcRunLog)
	}

	// 2. srsenb
	enbLogF, err := os.Create(enbRunLog)
	if err != nil {
		m.stopLocked()
		return known, err
	}
	enbCmd := exec.CommandContext(context.Background(), m.cfg.SrsENBBin, enbConf)
	enbCmd.Stdout = enbLogF
	enbCmd.Stderr = enbLogF
	enb, err := startManaged(enbCmd)
	_ = enbLogF.Close()
	if err != nil {
		m.stopLocked()
		return known, fmt.Errorf("start enb: %w", err)
	}
	m.enbCmd = enb
	if err := waitForInit(ctx, enbInitDelay); err != nil {
		m.stopLocked()
		return known, err
	}
	// Fail fast when the eNB died during init (typical: RF device vanished).
	// A live-but-slow eNB passes through; only a certain exit fails here.
	if !alive(enb) {
		m.stopLocked()
		return known, fmt.Errorf("enb exited early: %s", tailFile(enbRunLog, 5))
	}

	// 3. traffic capture on the SGi interface (best-effort).
	pcapPath := m.cfg.LogPath(m.cfg.PcapLTEData)
	pcapCmd := exec.CommandContext(context.Background(), m.cfg.TcpdumpBin,
		"-i", "srs_spgw_sgi", "-w", pcapPath)
	if pcap, err := startManaged(pcapCmd); err == nil {
		m.pcapCmd = pcap
	}

	m.startedAt = time.Now()
	m.lastStart = p
	m.runID = runID
	m.ueSnapshotPath = ueSnapshotPath
	m.lastBand = band
	m.bandKnown = known
	_ = m.SaveProfile(p) // best-effort: next /start {} reuses it
	return known, nil
}

// ProfilePath is /data/last_start.json: the last successful launch config.
// It survives container recreates via the /data volume (stateless service,
// stateful files).
func (m *Manager) ProfilePath() string { return filepath.Join(m.cfg.DataDir, "last_start.json") }

// SaveProfile persists effective start params while retaining the "auto"
// uplink policy rather than the interface it happened to resolve to.
func (m *Manager) SaveProfile(p StartParams) error {
	normalizeAPNMismatchPolicy(&p)
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.ProfilePath() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.ProfilePath())
}

// LoadProfile reads the persisted launch config. Empty network values from
// older profiles are upgraded in memory to the "auto" policy.
func (m *Manager) LoadProfile() (p StartParams, ok bool) {
	b, err := os.ReadFile(m.ProfilePath())
	if err != nil {
		return StartParams{}, false
	}
	var v StartParams
	if json.Unmarshal(b, &v) != nil {
		return StartParams{}, false
	}
	if strings.TrimSpace(v.Band) == "" || strings.TrimSpace(v.APN) == "" ||
		strings.TrimSpace(v.MCC) == "" || strings.TrimSpace(v.MNC) == "" {
		return StartParams{}, false
	}
	if strings.TrimSpace(v.Network) == "" {
		v.Network = "auto"
	}
	normalizeAPNMismatchPolicy(&v)
	if err := validateAPNMismatchPolicy(v.APNMismatchPolicy); err != nil {
		return StartParams{}, false
	}
	normalizeUEPolicy(&v)
	return v, true
}

// OverlayProfile fills every empty field of p from the saved profile.
// Returns false when no usable profile exists.
func (m *Manager) OverlayProfile(p *StartParams) bool {
	saved, ok := m.LoadProfile()
	if !ok {
		return false
	}
	if p.Band == "" {
		p.Band = saved.Band
	}
	if p.APN == "" {
		p.APN = saved.APN
	}
	if p.APNMismatchPolicy == "" {
		p.APNMismatchPolicy = saved.APNMismatchPolicy
	}
	if p.MCC == "" {
		p.MCC = saved.MCC
	}
	if p.MNC == "" {
		p.MNC = saved.MNC
	}
	if p.Network == "" {
		p.Network = saved.Network
	}
	if p.SDR == "" {
		p.SDR = saved.SDR
	}
	if p.DeviceArgs == "" {
		p.DeviceArgs = saved.DeviceArgs
	}
	if p.TxGain == nil {
		p.TxGain = saved.TxGain
	}
	if p.RxGain == nil {
		p.RxGain = saved.RxGain
	}
	if p.NPRB == nil {
		p.NPRB = saved.NPRB
	}
	if p.FullNetName == "" {
		p.FullNetName = saved.FullNetName
	}
	if p.ShortNetName == "" {
		p.ShortNetName = saved.ShortNetName
	}
	if p.DNS == "" {
		p.DNS = saved.DNS
	}
	if p.UESubnet == "" {
		p.UESubnet = saved.UESubnet
	}
	if p.UEAccess == "" {
		p.UEAccess = saved.UEAccess
	}
	return true
}

// Stop kills only children launched by this Manager and removes only network
// rules this Manager added. Same-name processes owned by other services are
// deliberately left alone.
func (m *Manager) Stop() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	was := m.hasLifecycleLocked()
	stopped := m.stopLocked()
	return was && stopped
}

// deleteNAT removes the single MASQUERADE rule recorded as ours.
func deleteNAT(iface, subnet string) error {
	return deleteOwnedRule(natRule(iface, subnet))
}

func (m *Manager) stopLocked() bool {
	for _, child := range []*managedChild{m.pcapCmd, m.enbCmd, m.epcCmd} {
		stopManaged(child, 2*time.Second)
	}
	if !alive(m.pcapCmd) {
		m.pcapCmd = nil
	}
	if !alive(m.enbCmd) {
		m.enbCmd = nil
	}
	if !alive(m.epcCmd) {
		m.epcCmd = nil
	}
	if m.natOwned && m.lastNetwork != "" {
		subnet := m.lastUESubnet
		if subnet == "" {
			subnet = defaultUESubnet
		}
		if deleteNAT(m.lastNetwork, subnet) == nil {
			m.natOwned = false
		}
	}
	m.forwardingOwned = cleanupForwarding(m.forwardingOwned)
	if !m.natOwned && len(m.forwardingOwned) == 0 {
		m.lastNetwork = ""
		m.lastUESubnet = ""
		m.lastUEAccess = ""
	}
	m.startedAt = time.Time{}
	m.runID = ""
	m.ueSnapshotPath = ""
	return !m.anyAliveLocked() && !m.natOwned && len(m.forwardingOwned) == 0
}

func (m *Manager) anyAliveLocked() bool {
	return alive(m.epcCmd) || alive(m.enbCmd) || alive(m.pcapCmd)
}

func (m *Manager) hasLifecycleLocked() bool {
	return m.epcCmd != nil || m.enbCmd != nil || m.pcapCmd != nil ||
		m.natOwned || len(m.forwardingOwned) != 0
}

// managedChild has exactly one Wait caller. done is the synchronization point
// for every status/start/stop path, avoiding races on exec.Cmd.ProcessState.
type managedChild struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func startManaged(cmd *exec.Cmd) (*managedChild, error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	child := &managedChild{cmd: cmd, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(child.done)
	}()
	return child, nil
}

// alive treats every completed Wait alike: zero, non-zero and signal exits
// are all stopped. ProcessState.Exited is intentionally not consulted because
// it is false for signal termination on Unix.
func alive(child *managedChild) bool {
	if child == nil || child.cmd == nil || child.cmd.Process == nil {
		return false
	}
	select {
	case <-child.done:
		return false
	default:
		return true
	}
}

func stopManaged(child *managedChild, timeout time.Duration) {
	if child == nil || !alive(child) {
		return
	}
	_ = child.cmd.Process.Kill()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-child.done:
	case <-timer.C:
	}
}

func waitForInit(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ueSubnet is retained as the canonical default for compatibility tests.
const ueSubnet = defaultUESubnet

// iptRule is one iptables rule (filter table when table == "").
type iptRule struct {
	table string
	chain string
	args  []string
}

// forwardRules returns the precise UE-subnet rules every /start must ensure:
// forwarding is limited to SGi<->uplink, unsolicited inbound packets are not
// accepted, and TCP MSS is clamped in both directions over the GTP path.
func forwardRules(uplink string, plans ...ueNetworkPlan) []iptRule {
	iface := uplink
	if iface == "" {
		iface = "UPLINK"
	}
	plan, _ := makeUENetworkPlan(defaultUESubnet, defaultUEAccess)
	if len(plans) != 0 {
		plan = plans[0]
	}
	subnet := plan.SubnetText
	policyTarget := "DROP"
	if plan.Access == "allow" {
		policyTarget = "ACCEPT"
	}
	return []iptRule{
		{"filter", "FORWARD", []string{"-i", sgiInterface, "-s", subnet,
			"-o", iface, "-j", "ACCEPT"}},
		{"filter", "FORWARD", []string{"-i", iface, "-o", sgiInterface,
			"-d", subnet, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"}},
		{"mangle", "FORWARD", []string{"-i", sgiInterface, "-s", subnet,
			"-o", iface, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
			"-j", "TCPMSS", "--clamp-mss-to-pmtu"}},
		{"mangle", "FORWARD", []string{"-i", iface, "-o", sgiInterface,
			"-d", subnet, "-p", "tcp",
			"--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}},
		// This rule is inserted last and therefore lands first in FORWARD.
		{"filter", "FORWARD", []string{"-i", sgiInterface, "-o", sgiInterface,
			"-s", subnet, "-d", subnet, "-j", policyTarget}},
	}
}

func natRule(iface string, subnets ...string) iptRule {
	subnet := defaultUESubnet
	if len(subnets) != 0 && subnets[0] != "" {
		subnet = subnets[0]
	}
	return iptRule{"nat", "POSTROUTING", []string{"-s", subnet,
		"-o", iface, "-j", "MASQUERADE"}}
}

func (r iptRule) commandArgs(op string) []string {
	a := []string{"-w", iptablesWaitSeconds}
	if r.table != "" && r.table != "filter" {
		a = append(a, "-t", r.table)
	}
	a = append(a, op, r.chain)
	return append(a, r.args...)
}

func (r iptRule) run(ctx context.Context, op string) error {
	_, err := runNetworkCommand(ctx, "iptables", r.commandArgs(op)...)
	return err
}

// ensureRule inserts a missing rule at the head of its chain. Inserting (not
// appending) is required because host and Docker policies may end in an early
// DROP or RETURN. A successful -C preserves pre-existing rules as unowned.
func ensureRule(ctx context.Context, r iptRule) (bool, error) {
	out, err := runNetworkCommand(ctx, "iptables", r.commandArgs("-C")...)
	if err == nil {
		return false, nil
	}
	if !isMissingRule(out, err) {
		return false, fmt.Errorf("check existing rule: %w", err)
	}
	if err := r.run(ctx, "-I"); err != nil {
		return false, err
	}
	return true, nil
}

// ensureForwarding adds missing rules and returns exactly the rules added by
// this invocation. Stop must not delete a pre-existing host rule.
func ensureForwarding(ctx context.Context, uplink string, plans ...ueNetworkPlan) ([]iptRule, error) {
	var added []iptRule
	rules := forwardRules(uplink, plans...)
	for i, r := range rules {
		var owned bool
		var err error
		if i == len(rules)-1 {
			err = r.run(ctx, "-I")
			owned = err == nil
		} else {
			owned, err = ensureRule(ctx, r)
		}
		if err != nil {
			return added, err
		}
		if owned {
			added = append(added, r)
		}
	}
	return added, nil
}

// cleanupForwarding removes only rules recorded as added by this Manager and
// returns rules that could not be removed. Each deletion gets its own bounded
// command so one xtables lock timeout does not discard later ownership.
func cleanupForwarding(owned []iptRule) []iptRule {
	var remaining []iptRule
	for i := len(owned) - 1; i >= 0; i-- {
		if err := deleteOwnedRule(owned[i]); err != nil {
			remaining = append(remaining, owned[i])
		}
	}
	// Preserve original insertion order for subsequent cleanup attempts.
	for i, j := 0, len(remaining)-1; i < j; i, j = i+1, j-1 {
		remaining[i], remaining[j] = remaining[j], remaining[i]
	}
	return remaining
}

func deleteOwnedRule(rule iptRule) error {
	if err := rule.run(context.Background(), "-D"); err == nil {
		return nil
	} else {
		out, checkErr := runNetworkCommand(context.Background(), "iptables", rule.commandArgs("-C")...)
		if isMissingRule(out, checkErr) {
			return nil
		}
		return err
	}
}

// ---- config rendering (ports of run.sh echo blocks) ----

type epcTmplData struct {
	MCC, MNC, APN             string
	FullNetName, ShortNetName string
	DNSAddr                   string
	SGIAddress                string
	UserDB, EPCPcap, EPCLog   string
}

type enbTmplData struct {
	MCC, MNC                  string
	DLEARFCN                  int
	TxGain, RxGain, NPRB      int
	DeviceName, DeviceArgs    string
	SibConf, RrConf, RbConf   string
	ENBPcap, S1APPcap, ENBLog string
}

var epcTmpl = template.Must(template.New("epc").Parse(`[mme]
mme_code = 0x1a
mme_group = 0x0001
tac = 0x0007
mcc = {{.MCC}}
mnc = {{.MNC}}
mme_bind_addr = 127.0.1.100
apn = {{.APN}}
full_net_name = {{.FullNetName}}
short_net_name = {{.ShortNetName}}
dns_addr = {{.DNSAddr}}
paging_timer = 2

[hss]
db_file = {{.UserDB}}

[spgw]
gtpu_bind_addr   = 127.0.1.100
sgi_if_addr      = {{.SGIAddress}}
sgi_if_name      = srs_spgw_sgi
max_paging_queue = 100

[pcap]
enable   = true
filename = {{.EPCPcap}}

[log]
all_level = info
all_hex_limit = 32
filename = {{.EPCLog}}
`))

var enbTmpl = template.Must(template.New("enb").Parse(`[enb]
mcc = {{.MCC}}
mnc = {{.MNC}}
mme_addr = 127.0.1.100
gtp_bind_addr = 127.0.1.1
s1c_bind_addr = 127.0.1.1
n_prb = {{.NPRB}}

[enb_files]
sib_config = {{.SibConf}}
rr_config = {{.RrConf}}
rb_config = {{.RbConf}}

[rf]
dl_earfcn = {{.DLEARFCN}}
tx_gain = {{.TxGain}}
rx_gain = {{.RxGain}}
device_name = {{.DeviceName}}
device_args = {{.DeviceArgs}}

[pcap]
enable = true
filename = {{.ENBPcap}}
s1ap_enable = true
s1ap_filename = {{.S1APPcap}}
[log]
all_level = warning
all_hex_limit = 0
phy_hex_limit = 0
rf_level = info
rrc_level = info
s1ap_level = info
filename = {{.ENBLog}}
file_max_size = -1

[gui]
enable = false

[scheduler]

[embms]

[channel.dl]

[channel.dl.awgn]

[channel.dl.fading]

[channel.dl.delay]

[channel.dl.rlf]

[channel.dl.hst]

[channel.ul]

[channel.ul.awgn]

[channel.ul.fading]

[channel.ul.delay]

[channel.ul.rlf]

[channel.ul.hst]

[expert]
`))

// rrTmplData carries the per-band cell earfcns into rr.conf.
// ul_earfcn is always explicit: srsRAN_4G's internal UL derivation fails for
// TDD bands, and explicit values also pin FDD correctly (verified live).
type rrTmplData struct {
	DLEARFCN int
	ULEARFCN int
}

// rrTmpl mirrors configs/rr.conf (upstream srsRAN_4G rr.conf.example).
// configs/rr.conf is the checked-in reference render (band 7 values).
var rrTmpl = template.Must(template.New("rr").Parse(`mac_cnfg =
{
  phr_cnfg =
  {
    dl_pathloss_change = "dB3"; // Valid: 1, 3, 6 or INFINITY
    periodic_phr_timer = 50;
    prohibit_phr_timer = 0;
  };
  ulsch_cnfg =
  {
    max_harq_tx = 4;
    periodic_bsr_timer = 20; // in ms
    retx_bsr_timer = 320;   // in ms
  };

  time_alignment_timer = -1; // -1 is infinity
};

phy_cnfg =
{
  phich_cnfg =
  {
    duration  = "Normal";
    resources = "1/6";
  };

  pusch_cnfg_ded =
  {
    beta_offset_ack_idx = 6;
    beta_offset_ri_idx  = 6;
    beta_offset_cqi_idx = 6;
  };

  // PUCCH-SR resources are scheduled on time-frequeny domain first, then multiplexed in the same resource.
  sched_request_cnfg =
  {
    dsr_trans_max = 64;
    period = 20;          // in ms
    //subframe = [1, 11]; // Optional vector of subframe indices allowed for SR transmissions (default uses all)
    nof_prb = 1;          // number of PRBs on each extreme used for SR (total prb is twice this number)
  };
  cqi_report_cnfg =
  {
    mode = "periodic";
    simultaneousAckCQI = true;
    period = 40;                   // in ms
    //subframe = [0, 10, 20, 30];  // Optional vector of subframe indices every period where CQI resources will be allocated (default uses all)
    m_ri = 8; // RI period in CQI period
    //subband_k = 1; // If enabled and > 0, configures sub-band CQI reporting and defines K (see 36.213 7.2.2). If disabled, configures wideband CQI
  };
};

cell_list =
(
  {
    // rf_port = 0;
    cell_id = 0x01;
    tac = 0x0007;
    pci = 1;
    // root_seq_idx = 204;
    dl_earfcn = {{.DLEARFCN}};
    ul_earfcn = {{.ULEARFCN}};
    ho_active = false;
    //meas_gap_period = 0; // 0 (inactive), 40 or 80
    //meas_gap_offset_subframe = [6, 12, 18, 24, 30];
    // target_pusch_sinr = -1;
    // target_pucch_sinr = -1;
    // enable_phr_handling = false;
    // min_phr_thres = 0;
    // allowed_meas_bw = 6;
    // t304 = 2000; // in msec. possible values: 50, 100, 150, 200, 500, 1000, 2000
    // tx_gain = 20.0; // in dB. This gain is set by scaling the source signal.

    // CA cells
    scell_list = (
      // {cell_id = 0x02; cross_carrier_scheduling = false; scheduling_cell_id = 0x02; ul_allowed = true}
    )

    // Cells available for handover
    meas_cell_list =
    (
      {
        eci = 0x19C02;
        dl_earfcn = 2850;
        pci = 2;
        //direct_forward_path_available = false;
        //allowed_meas_bw = 6;
        //cell_individual_offset = 0;
      }
    );

    // Select measurement report configuration (all reports are combined with all measurement objects)
    meas_report_desc =
    (
        {
          eventA = 3
          a3_offset = 6;
          hysteresis = 0;
          time_to_trigger = 480;
          trigger_quant = "RSRP";
          max_report_cells = 1;
          report_interv = 120;
          report_amount = 1;
        }
    );
    meas_quant_desc = {
        // averaging filter coefficient
        rsrq_config = 4;
        rsrp_config = 4;
     };
  }
  // Add here more cells
);

nr_cell_list =
(
  // no NR cells
);
`))

func (m *Manager) renderAll(p StartParams, band BandInfo, devName, devArgs string, tx, rx, nprb int) error {
	normalizeUEPolicy(&p)
	uePlan, err := makeUENetworkPlan(p.UESubnet, p.UEAccess)
	if err != nil {
		return err
	}
	userDB := m.cfg.UserDBPath()
	if err := ensureUserDB(userDB, m.cfg.ConfDir); err != nil {
		return err
	}
	// Copy static sib/rb if missing (from bundled configs dir).
	// rr.conf is rendered below on every start (cell earfcns depend on band).
	for _, name := range []string{"sib.conf", "rb.conf"} {
		dst := filepath.Join(m.cfg.ConfDir, name)
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			for _, cand := range []string{filepath.Join("configs", name), filepath.Join("/app/configs", name)} {
				if b, err := os.ReadFile(cand); err == nil {
					_ = os.WriteFile(dst, b, 0o644)
					break
				}
			}
		}
	}
	epcData := epcTmplData{
		MCC: p.MCC, MNC: p.MNC, APN: p.APN,
		FullNetName: p.FullNetName, ShortNetName: p.ShortNetName,
		DNSAddr: p.DNS, SGIAddress: uePlan.SGIAddress,
		UserDB:  userDB,
		EPCPcap: m.cfg.LogPath(m.cfg.PcapEPC),
		EPCLog:  m.cfg.LogPath(m.cfg.EPCLogName),
	}
	enbData := enbTmplData{
		MCC: p.MCC, MNC: p.MNC,
		DLEARFCN: band.DLEARFCN, TxGain: tx, RxGain: rx, NPRB: nprb,
		DeviceName: devName, DeviceArgs: devArgs,
		SibConf:  filepath.Join(m.cfg.ConfDir, "sib.conf"),
		RrConf:   filepath.Join(m.cfg.ConfDir, "rr.conf"),
		RbConf:   filepath.Join(m.cfg.ConfDir, "rb.conf"),
		ENBPcap:  m.cfg.LogPath(m.cfg.PcapENB),
		S1APPcap: m.cfg.LogPath(m.cfg.PcapS1AP),
		ENBLog:   m.cfg.LogPath(m.cfg.ENBLogName),
	}
	if err := writeTmpl(filepath.Join(m.cfg.ConfDir, "epc_run.conf"), epcTmpl, epcData); err != nil {
		return err
	}
	if err := writeTmpl(filepath.Join(m.cfg.ConfDir, "enb_run.conf"), enbTmpl, enbData); err != nil {
		return err
	}
	return writeTmpl(filepath.Join(m.cfg.ConfDir, "rr.conf"), rrTmpl,
		rrTmplData{DLEARFCN: band.DLEARFCN, ULEARFCN: band.ULEARFCN})
}

// ensureUserDB serializes only the first-run check+seed mutation with API
// replacement, SIM programming and subscriber append operations.
func ensureUserDB(userDB, confDir string) error {
	subscriber.Mutex.Lock()
	defer subscriber.Mutex.Unlock()

	if _, err := os.Stat(userDB); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat user database: %w", err)
	}
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	for _, cand := range []string{"configs/user_db.csv.example", "/app/configs/user_db.csv.example", "user_db.csv.example"} {
		if b, err := os.ReadFile(cand); err == nil {
			if err := os.WriteFile(userDB, b, 0o644); err != nil {
				return fmt.Errorf("seed user database: %w", err)
			}
			return nil
		}
	}
	header := []byte("# Name,Auth,IMSI,Key,OP_Type,OP/OPc,AMF,SQN,QCI,IP_alloc\n")
	if err := os.WriteFile(userDB, header, 0o644); err != nil {
		return fmt.Errorf("create user database: %w", err)
	}
	return nil
}

// tailFile returns the last n non-empty lines of path joined for log snippets.
func tailFile(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, strings.TrimSpace(line))
		if len(out) > n {
			out = out[1:]
		}
	}
	s := strings.Join(out, " | ")
	if len(s) > 500 {
		s = s[len(s)-500:]
	}
	return s
}

func writeTmpl(path string, t *template.Template, data any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.Execute(f, data)
}
