// Package lte manages the srsRAN_4G EPC+eNB lifecycle (stateless tool side).
package lte

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/addxemmm/lte-system/internal/config"
	"github.com/addxemmm/lte-system/internal/sdr"
	"github.com/addxemmm/lte-system/internal/sysop"
)

// StartParams mirrors POST /start (legacy fields + SDR extensions).
type StartParams struct {
	Band       string `json:"band"`
	APN        string `json:"apn"`
	MCC        string `json:"mcc"`
	MNC        string `json:"mnc"`
	Network    string `json:"network"`
	SDR        string `json:"sdr"`         // "uhd"|"bladerf"|"zmq"|"auto"|""
	DeviceArgs string `json:"device_args"` // optional UHD/bladeRF args
	TxGain     *int   `json:"tx_gain"`
	RxGain     *int   `json:"rx_gain"`
	NPRB       *int   `json:"n_prb"`
	// Operator name shown on the UE (NITZ via EMM Information).
	// Empty = server default (legacy display "srsRAN").
	FullNetName  string `json:"full_net_name"`
	ShortNetName string `json:"short_net_name"`
}

// Validate checks legacy "incomplete parameters" + new field formats.
func (p StartParams) Validate() error {
	if strings.TrimSpace(p.Band) == "" || strings.TrimSpace(p.APN) == "" ||
		strings.TrimSpace(p.MCC) == "" || strings.TrimSpace(p.MNC) == "" ||
		strings.TrimSpace(p.Network) == "" {
		return fmt.Errorf("incomplete parameters")
	}
	if len(p.MCC) != 3 || !isDigits(p.MCC) {
		return fmt.Errorf("mcc must be 3 digits")
	}
	if !(len(p.MNC) == 2 || len(p.MNC) == 3) || !isDigits(p.MNC) {
		return fmt.Errorf("mnc must be 2 or 3 digits")
	}
	if strings.ContainsAny(p.APN, " \t\n\r\"';&|<>$`\\") {
		return fmt.Errorf("apn contains illegal characters")
	}
	if strings.ContainsAny(p.Network, " \t\n\r\"';&|<>$`\\") {
		return fmt.Errorf("network contains illegal characters")
	}
	if err := validateNetName(p.FullNetName); err != nil {
		return fmt.Errorf("full_net_name: %w", err)
	}
	if err := validateNetName(p.ShortNetName); err != nil {
		return fmt.Errorf("short_net_name: %w", err)
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

// Manager owns EPC/eNB/tcpdump processes and rendered configs.
type Manager struct {
	cfg config.Config
	mu  sync.Mutex

	epcCmd  *exec.Cmd
	enbCmd  *exec.Cmd
	pcapCmd *exec.Cmd

	startedAt time.Time
	lastStart StartParams
	lastBand  BandInfo
	bandKnown bool
}

// New creates a Manager.
func New(cfg config.Config) *Manager { return &Manager{cfg: cfg} }

// Status is the machine-readable state for /status and /healthz.
type Status struct {
	Running   bool       `json:"running"`
	EPC       bool       `json:"epc"`
	ENB       bool       `json:"enb"`
	Pcap      bool       `json:"pcap"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	Band      string     `json:"band,omitempty"`
	APN       string     `json:"apn,omitempty"`
	NetName   string     `json:"net_name,omitempty"`
}

// IsRunning reports live state (managed procs OR external same-name procs).
func (m *Manager) IsRunning() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	epc := sysop.Running("srsepc") || (m.epcCmd != nil && m.epcCmd.Process != nil)
	enb := sysop.Running("srsenb") || (m.enbCmd != nil && m.enbCmd.Process != nil)
	pcap := sysop.Running("tcpdump")
	st := Status{EPC: epc, ENB: enb, Pcap: pcap, Running: epc || enb}
	if st.Running && !m.startedAt.IsZero() {
		t := m.startedAt
		st.StartedAt = &t
		st.Band = m.lastStart.Band
		st.APN = m.lastStart.APN
		st.NetName = m.lastStart.FullNetName
	}
	return st
}

// Start renders configs and launches srsepc -> iptables -> srsenb -> tcpdump.
// It mirrors legacy run.sh but fixes the hard-coded srsenb path and adds SDR choice.
func (m *Manager) Start(ctx context.Context, p StartParams) (bandKnown bool, err error) {
	if err := p.Validate(); err != nil {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if sysop.Running("srsepc") || sysop.Running("srsenb") {
		return false, fmt.Errorf("is running")
	}
	if err := m.cfg.EnsureDirs(); err != nil {
		return false, err
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
	}
	if p.RxGain != nil {
		rxGain = *p.RxGain
	}
	if p.NPRB != nil {
		nPRB = *p.NPRB
	}
	// Resolve display network names (request overrides server defaults).
	if p.FullNetName == "" {
		p.FullNetName = m.cfg.DefaultFullNetName
	}
	if p.ShortNetName == "" {
		p.ShortNetName = m.cfg.DefaultShortNetName
	}
	devName, devArgs := sdr.SelectArgs(p.SDR, p.DeviceArgs, det, m.cfg)

	if err := m.renderAll(p, band, devName, devArgs, txGain, rxGain, nPRB); err != nil {
		return known, err
	}

	epcConf := filepath.Join(m.cfg.ConfDir, "epc_run.conf")
	enbConf := filepath.Join(m.cfg.ConfDir, "enb_run.conf")
	epcRunLog := filepath.Join(m.cfg.LogDir, "epc_run.log")
	enbRunLog := filepath.Join(m.cfg.LogDir, "enb_run.log")

	// 1. srsepc
	epcLogF, err := os.Create(epcRunLog)
	if err != nil {
		return known, err
	}
	epcCmd := exec.CommandContext(context.Background(), m.cfg.SrsEPCBin, epcConf)
	epcCmd.Stdout = epcLogF
	epcCmd.Stderr = epcLogF
	if err := epcCmd.Start(); err != nil {
		epcLogF.Close()
		return known, fmt.Errorf("start epc: %w", err)
	}
	m.epcCmd = epcCmd
	time.Sleep(3 * time.Second)
	if epcCmd.ProcessState != nil && epcCmd.ProcessState.Exited() {
		epcLogF.Close()
		return known, fmt.Errorf("epc exited early, see %s", epcRunLog)
	}

	// 2. NAT for UE subnet via the uplink interface (legacy iptables rule).
	_ = exec.Command("iptables", "-t", "nat", "-C", "POSTROUTING",
		"-s", "172.16.0.1/24", "-o", p.Network, "-j", "MASQUERADE").Run()
	_ = exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING",
		"-s", "172.16.0.1/24", "-o", p.Network, "-j", "MASQUERADE").Run()

	// 3. srsenb
	enbLogF, err := os.Create(enbRunLog)
	if err != nil {
		m.killLocked()
		return known, err
	}
	enbCmd := exec.CommandContext(context.Background(), m.cfg.SrsENBBin, enbConf)
	enbCmd.Stdout = enbLogF
	enbCmd.Stderr = enbLogF
	if err := enbCmd.Start(); err != nil {
		enbLogF.Close()
		m.killLocked()
		return known, fmt.Errorf("start enb: %w", err)
	}
	m.enbCmd = enbCmd
	time.Sleep(3 * time.Second)

	// 4. traffic capture on the SGi interface (best-effort).
	pcapPath := m.cfg.LogPath(m.cfg.PcapLTEData)
	pcapCmd := exec.CommandContext(context.Background(), m.cfg.TcpdumpBin,
		"-i", "srs_spgw_sgi", "-w", pcapPath)
	_ = pcapCmd.Start()
	m.pcapCmd = pcapCmd

	m.startedAt = time.Now()
	m.lastStart = p
	m.lastBand = band
	m.bandKnown = known
	_ = ctx
	return known, nil
}

// Stop kills tcpdump + srsenb + srsepc and removes the NAT rule we added.
func (m *Manager) Stop() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	was := sysop.Running("srsepc") || sysop.Running("srsenb") || sysop.Running("tcpdump")
	m.killLocked()
	// Remove our NAT rule (ignore errors; interface may be gone).
	if m.lastStart.Network != "" {
		_ = exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING",
			"-s", "172.16.0.1/24", "-o", m.lastStart.Network, "-j", "MASQUERADE").Run()
	}
	time.Sleep(time.Second)
	still := sysop.Running("srsepc") || sysop.Running("srsenb") || sysop.Running("tcpdump")
	return was && !still || (was && m.epcCmd == nil && m.enbCmd == nil && !still)
}

func (m *Manager) killLocked() {
	for _, cmd := range []*exec.Cmd{m.pcapCmd, m.enbCmd, m.epcCmd} {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
			reap(cmd)
		}
	}
	m.pcapCmd = nil
	m.enbCmd = nil
	m.epcCmd = nil
	sysop.KillAll("tcpdump", 2*time.Second)
	sysop.KillAll("srsenb", 3*time.Second)
	sysop.KillAll("srsepc", 3*time.Second)
	m.startedAt = time.Time{}
}

// reap waits for a killed child so it doesn't linger as <defunct>.
// (Unreaped zombies made PIDs lie until the zombie-aware check was added;
// belt and suspenders.)
func reap(cmd *exec.Cmd) {
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

// ---- config rendering (ports of run.sh echo blocks) ----

type epcTmplData struct {
	MCC, MNC, APN                    string
	FullNetName, ShortNetName        string
	UserDB, EPCPcap, EPCLog           string
}

type enbTmplData struct {
	MCC, MNC                   string
	DLEARFCN                   int
	TxGain, RxGain, NPRB       int
	DeviceName, DeviceArgs     string
	SibConf, RrConf, RbConf     string
	ENBPcap, S1APPcap, ENBLog  string
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
dns_addr = 8.8.8.8
paging_timer = 2

[hss]
db_file = {{.UserDB}}

[spgw]
gtpu_bind_addr   = 127.0.1.100
sgi_if_addr      = 172.16.0.1
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
all_level = info
all_hex_limit = 32
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
	userDB := m.cfg.UserDBPath()
	// Ensure a user_db.csv exists (copy example on first run).
	if _, err := os.Stat(userDB); os.IsNotExist(err) {
		seeded := false
		for _, cand := range []string{"configs/user_db.csv.example", "/app/configs/user_db.csv.example", "user_db.csv.example"} {
			if b, err := os.ReadFile(cand); err == nil {
				_ = os.MkdirAll(m.cfg.ConfDir, 0o755)
				_ = os.WriteFile(userDB, b, 0o644)
				seeded = true
				break
			}
		}
		if !seeded {
			_ = os.MkdirAll(m.cfg.ConfDir, 0o755)
			_ = os.WriteFile(userDB, []byte("# Name,Auth,IMSI,Key,OP_Type,OP/OPc,AMF,SQN,QCI,IP_alloc\n"), 0o644)
		}
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
		UserDB: userDB,
		EPCPcap: m.cfg.LogPath(m.cfg.PcapEPC),
		EPCLog:  m.cfg.LogPath(m.cfg.EPCLogName),
	}
	enbData := enbTmplData{
		MCC: p.MCC, MNC: p.MNC,
		DLEARFCN: band.DLEARFCN, TxGain: tx, RxGain: rx, NPRB: nprb,
		DeviceName: devName, DeviceArgs: devArgs,
		SibConf: filepath.Join(m.cfg.ConfDir, "sib.conf"),
		RrConf:  filepath.Join(m.cfg.ConfDir, "rr.conf"),
		RbConf:   filepath.Join(m.cfg.ConfDir, "rb.conf"),
		ENBPcap: m.cfg.LogPath(m.cfg.PcapENB),
		S1APPcap: m.cfg.LogPath(m.cfg.PcapS1AP),
		ENBLog:  m.cfg.LogPath(m.cfg.ENBLogName),
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

func writeTmpl(path string, t *template.Template, data any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.Execute(f, data)
}
