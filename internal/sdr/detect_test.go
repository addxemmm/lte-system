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
