package sdr

import (
	"testing"

	"github.com/addxemmm/lte-system/internal/config"
)

func TestSelectArgs_Explicit(t *testing.T) {
	c := config.Default()
	if n, _ := SelectArgs("bladerf", "auto", Info{}, c); n != "bladerf" {
		t.Fatalf("want bladerf got %s", n)
	}
	if n, _ := SelectArgs("uhd", "auto", Info{}, c); n != "uhd" {
		t.Fatalf("want uhd got %s", n)
	}
	if n, _ := SelectArgs("zmq", "auto", Info{}, c); n != "zmq" {
		t.Fatalf("want zmq got %s", n)
	}
}

func TestSelectArgs_AutoPrefersDetected(t *testing.T) {
	c := config.Default()
	if n, _ := SelectArgs("auto", "auto", Info{BladeRF: true}, c); n != "bladerf" {
		t.Fatalf("auto should prefer bladerf, got %s", n)
	}
	if n, _ := SelectArgs("", "auto", Info{UHD_B210: true}, c); n != "auto" {
		t.Fatalf("uhd uses auto driver, got %s", n)
	}
}

func TestSelectArgs_CustomArgs(t *testing.T) {
	c := config.Default()
	_, a := SelectArgs("uhd", "num_recv_frames=64", Info{}, c)
	if a != "num_recv_frames=64" {
		t.Fatalf("custom args lost: %s", a)
	}
}

func TestSelectArgs_B210AutoTuning(t *testing.T) {
	c := config.Default()
	// Detected B210 + default args -> proven VM-USB-stable tuning.
	_, a := SelectArgs("auto", "auto", Info{UHD_B210: true}, c)
	if a != B210AutoArgs {
		t.Fatalf("want B210 auto args, got %q", a)
	}
	_, a = SelectArgs("uhd", "auto", Info{UHD_B210: true}, c)
	if a != B210AutoArgs {
		t.Fatalf("want B210 auto args, got %q", a)
	}
	// Explicit args always win over auto tuning.
	_, a = SelectArgs("uhd", "num_recv_frames=64", Info{UHD_B210: true}, c)
	if a != "num_recv_frames=64" {
		t.Fatalf("explicit args must win, got %q", a)
	}
	// No B210 detected -> untouched "auto".
	_, a = SelectArgs("auto", "auto", Info{}, c)
	if a != "auto" {
		t.Fatalf("want auto, got %q", a)
	}
	// bladeRF must never receive B210 USB tuning.
	_, a = SelectArgs("bladerf", "auto", Info{UHD_B210: true, BladeRF: true}, c)
	if a != "auto" {
		t.Fatalf("bladerf must keep auto args, got %q", a)
	}
}
