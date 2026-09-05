// Package sdr detects attached SDR hardware without shell injection.
package sdr

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/addxemmm/lte-system/internal/config"
)

// Info describes detected radios.
type Info struct {
	UHD_B210    bool   `json:"uhd_b210"`
	BladeRF     bool   `json:"bladerf"`
	ACR1281     bool   `json:"acr1281"`
	UHDRaw      string `json:"uhd_raw,omitempty"`
	BladeRFRaw  string `json:"bladerf_raw,omitempty"`
	USBACRRaw   string `json:"usb_acr_raw,omitempty"`
}

// Detect probes UHD + bladeRF + ACR1281. Each probe has a short timeout and
// never fails hard (returns what it found; empty = not present).
func Detect() Info {
	var in Info
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	if out, err := exec.CommandContext(ctx, "uhd_find_devices").CombinedOutput(); err == nil {
		in.UHDRaw = string(out)
	} else {
		// uhd_find_devices exits non-zero when nothing found but still prints;
		// keep output for matching.
		in.UHDRaw = string(out)
	}
	upper := strings.ToUpper(in.UHDRaw)
	if strings.Contains(upper, "B210") || strings.Contains(upper, "B200") {
		in.UHD_B210 = true
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	if out, err := exec.CommandContext(ctx2, "bladeRF-cli", "-e", "info").CombinedOutput(); err == nil {
		in.BladeRFRaw = string(out)
		l := strings.ToLower(in.BladeRFRaw)
		if strings.Contains(l, "serial") || strings.Contains(l, "fpga") || strings.Contains(l, "bladerf") {
			in.BladeRF = true
		}
	}

	ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel3()
	if out, err := exec.CommandContext(ctx3, "lsusb").CombinedOutput(); err == nil {
		s := string(out)
		if strings.Contains(s, "ACR128") {
			in.ACR1281 = true
			for _, line := range strings.Split(s, "\n") {
				if strings.Contains(line, "ACR128") {
					in.USBACRRaw = line
					break
				}
			}
		}
	}
	return in
}

// SelectArgs resolves device_name/device_args for enb.conf [rf].
// sdrWanted comes from /start "sdr" field: "uhd" | "bladerf" | "zmq" | "auto" | "".
func SelectArgs(sdrWanted string, customArgs string, det Info, defaults config.Config) (deviceName, deviceArgs string) {
	if customArgs != "" && customArgs != "auto" {
		deviceArgs = customArgs
	} else {
		deviceArgs = defaults.DefaultDeviceArgs
		if deviceArgs == "" {
			deviceArgs = "auto"
		}
	}
	switch sdrWanted {
	case "uhd":
		return "uhd", deviceArgs
	case "bladerf":
		return "bladerf", deviceArgs
	case "zmq":
		return "zmq", deviceArgs
	default:
		// auto: prefer detected hardware, fall back to configured default
		if det.BladeRF && !det.UHD_B210 {
			return "bladerf", deviceArgs
		}
		if defaults.DefaultSDR == config.SDRBladeRF {
			return "bladerf", deviceArgs
		}
		if defaults.DefaultSDR == config.SDRZMQ {
			return "zmq", deviceArgs
		}
		return "auto", deviceArgs
	}
}
