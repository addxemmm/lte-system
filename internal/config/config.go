// Package config loads stateless tool configuration from env + yaml.
// No database: all state lives in files under DataDir + in-memory manager.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SDRType selects the RF driver.
type SDRType string

const (
	SDRUHD     SDRType = "uhd"     // USRP B210 (stock + BlackSDR/compatible)
	SDRBladeRF SDRType = "bladerf" // bladeRF x40/xA4/2.0 micro
	SDRZMQ     SDRType = "zmq"     // RF-less test (srsRAN zmq virtual radio)
	SDRAuto    SDRType = "auto"
)

// Config is the full server configuration.
type Config struct {
	ListenAddr string `yaml:"listen_addr"`
	DataDir    string `yaml:"data_dir"` // e.g. /data : conf, log, pcap live here
	ConfDir    string `yaml:"conf_dir"` // rendered srsRAN conf dir (default DataDir/conf)
	LogDir     string `yaml:"log_dir"`  // default DataDir/log

	// Binaries (srsRAN_4G install paths)
	SrsEPCBin string `yaml:"srsepc_bin"`
	SrsENBBin string `yaml:"srsenb_bin"`

	// Tools
	TcpdumpBin string `yaml:"tcpdump_bin"`
	TsharkBin  string `yaml:"tshark_bin"`
	HashcatBin string `yaml:"hashcat_bin"`
	PySimDir   string `yaml:"pysim_dir"` // dir containing pySim-prog.py / pySim-read.py

	// Defaults for /start
	DefaultSDR        SDRType `yaml:"default_sdr"`
	DefaultDeviceArgs string  `yaml:"default_device_args"`
	DefaultTxGain     int     `yaml:"default_tx_gain"`
	DefaultRxGain     int     `yaml:"default_rx_gain"`
	DefaultNRB        int     `yaml:"default_n_prb"` // 25 = VM-USB safe 5MHz; 50/100 on bare metal
	// DNS server handed to UEs via PCO. Default 8.8.8.8; if the uplink
	// filters public DNS, point at the LAN resolver (e.g. the gateway).
	DefaultDNS string `yaml:"default_dns"`
	// Operator display name (NITZ) when /start omits full/short_net_name.
	DefaultFullNetName  string `yaml:"default_full_net_name"`
	DefaultShortNetName string `yaml:"default_short_net_name"`

	// Defaults for /writesim (flexible card programming)
	Sim SimDefaults `yaml:"sim_defaults"`

	// Pcap / log filenames (relative to LogDir)
	PcapLTEData string `yaml:"pcap_lte_data"`
	PcapENB     string `yaml:"pcap_enb"`
	PcapS1AP    string `yaml:"pcap_s1ap"`
	PcapEPC     string `yaml:"pcap_epc"`
	EPCLogName  string `yaml:"epc_log_name"`
	ENBLogName  string `yaml:"enb_log_name"`

	// Upload limits
	MaxUploadBytes int64 `yaml:"max_upload_bytes"`
}

// SimDefaults holds the fallback values used when /writesim omits fields.
// They match the legacy hard-coded values (Ki/OPc/ADM) so old clients keep working.
type SimDefaults struct {
	Ki    string `yaml:"ki"`    // 32 hex, legacy 00112233445566778899aabbccddeeff
	OPc   string `yaml:"opc"`   // 32 hex, legacy 63bfa50ee6523365ff14c1f45f88737d
	OP    string `yaml:"op"`    // optional alternative to opc (mutually exclusive)
	OPType string `yaml:"op_type"` // "opc" or "op", default "opc"
	Auth  string `yaml:"auth"`  // "mil" or "xor", default "mil"
	AMF   string `yaml:"amf"`   // 4 hex, default "8001" (matches proven ue3 row)
	ACC   string `yaml:"acc"`   // 4 hex, default "FFFF"
	ADM   string `yaml:"adm"`   // hex ascii, default "3030303030303030"
	SPN   string `yaml:"spn"`   // default "LTESystem"
	ICCID string `yaml:"iccid"` // default fixed test iccid; "auto" = keep card factory value
	Card  string `yaml:"card"`  // pysim card type, default "testsim"
	QCI   int    `yaml:"qci"`
}

// Default returns sane defaults matching legacy behavior + srsRAN_4G paths.
func Default() Config {
	return Config{
		ListenAddr:        ":8081",
		DataDir:           "/data",
		SrsEPCBin:         "srsepc",
		SrsENBBin:         "srsenb",
		TcpdumpBin:        "tcpdump",
		TsharkBin:         "tshark",
		HashcatBin:        "hashcat",
		PySimDir:          "/opt/pysim",
		DefaultSDR:        SDRAuto,
		DefaultDeviceArgs: "auto",
		DefaultTxGain:     80,
		DefaultRxGain:     40,
		DefaultNRB:        25,
		DefaultDNS:        "8.8.8.8",
		DefaultFullNetName:  "srsRAN",
		DefaultShortNetName: "srsRAN",
		Sim: SimDefaults{
			Ki:     "00112233445566778899aabbccddeeff",
			OPc:    "63bfa50ee6523365ff14c1f45f88737d",
			OPType: "opc",
		Auth:   "mil",
		AMF:    "8001",
			ACC:    "FFFF",
			ADM:    "3030303030303030",
			SPN:    "LTESystem",
			ICCID:  "89860123456789012345",
			Card:   "testsim",
			QCI:    7,
		},
		PcapLTEData:    "lte_data.pcap",
		PcapENB:        "srsLTE_enb.pcap",
		PcapS1AP:       "srsLTE_enb_s1ap.pcap",
		PcapEPC:        "srsLTE_epc.pcap",
		EPCLogName:     "srsLTE_epc.log",
		ENBLogName:     "srsLTE_enb.log",
		MaxUploadBytes: 8 << 20, // 8 MiB
	}
}

// Load reads YAML file if present, then overlays env vars.
// Env: LTE_LISTEN, LTE_DATA_DIR, LTE_SRSEPC_BIN, LTE_SRSENB_BIN, LTE_PYSIM_DIR.
func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return cfg, fmt.Errorf("read config %s: %w", path, err)
			}
		} else if len(b) > 0 {
			if err := yaml.Unmarshal(b, &cfg); err != nil {
				return cfg, fmt.Errorf("parse config %s: %w", path, err)
			}
		}
	}
	if v := os.Getenv("LTE_LISTEN"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("LTE_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("LTE_SRSEPC_BIN"); v != "" {
		cfg.SrsEPCBin = v
	}
	if v := os.Getenv("LTE_SRSENB_BIN"); v != "" {
		cfg.SrsENBBin = v
	}
	if v := os.Getenv("LTE_PYSIM_DIR"); v != "" {
		cfg.PySimDir = v
	}
	if cfg.ConfDir == "" {
		cfg.ConfDir = filepath.Join(cfg.DataDir, "conf")
	}
	if cfg.LogDir == "" {
		cfg.LogDir = filepath.Join(cfg.DataDir, "log")
	}
	return cfg, nil
}

// EnsureDirs creates DataDir/conf/log.
func (c Config) EnsureDirs() error {
	for _, d := range []string{c.DataDir, c.ConfDir, c.LogDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}

// UserDBPath returns the live HSS csv path.
func (c Config) UserDBPath() string { return filepath.Join(c.ConfDir, "user_db.csv") }

// WordlistPath returns the live hashcat wordlist path.
func (c Config) WordlistPath() string { return filepath.Join(c.DataDir, "wordlist.list") }

// LogPath joins a log/pcap filename under LogDir.
func (c Config) LogPath(name string) string { return filepath.Join(c.LogDir, name) }
