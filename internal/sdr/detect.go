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
		if isReaderLsusb(s) {
			in.ACR1281 = true
			for _, line := range strings.Split(s, "\n") {
				if isReaderLsusb(line) {
					in.USBACRRaw = line
					break
				}
			}
		}
	}
	return in
}

// isReaderLsusb matches ACS smartcard readers (ACR128* and any 072f VID
// device: ACR122U/ACR125x/ACR1281U...). Case-insensitive.
func isReaderLsusb(s string) bool {
	up := strings.ToUpper(s)
	return strings.Contains(up, "ACR128") || strings.Contains(up, "072F")
}

// B210AutoArgs are proven USB-stable UHD args for B210 on a VM host
// (kills the "Tx while waiting for EOB, timed out" storm at 5-10MHz).
const B210AutoArgs = "recv_frame_size=9232,send_frame_size=9232,num_recv_frames=64,num_send_frames=64"

// SelectArgs resolves device_name/device_args for enb.conf [rf].
// sdrWanted comes from /start "sdr" field: "uhd" | "bladerf" | "zmq" | "auto" | "".
//
// Rules: explicit non-"auto" args always win. Otherwise a detected B210 on a
// UHD/auto driver gets B210AutoArgs (VM-USB safe); bladeRF/ZMQ keep "auto"
// (B210 USB tuning must never leak into other drivers).
func SelectArgs(sdrWanted string, customArgs string, det Info, defaults config.Config) (deviceName, deviceArgs string) {
	explicit := customArgs != "" && customArgs != "auto"
	if explicit {
		deviceArgs = customArgs
	} else {
		deviceArgs = defaults.DefaultDeviceArgs
		if deviceArgs == "" {
			deviceArgs = "auto"
		}
	}
	switch sdrWanted {
	case "uhd":
		if !explicit && deviceArgs == "auto" && det.UHD_B210 {
			deviceArgs = B210AutoArgs
		}
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
		if !explicit && deviceArgs == "auto" && det.UHD_B210 {
			deviceArgs = B210AutoArgs
		}
		return "auto", deviceArgs
	}
}
