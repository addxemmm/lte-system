package sdr

import "testing"

func TestIsReaderLsusb(t *testing.T) {
	if !isReaderLsusb("Bus 001 Device 002: ID 072f:2209 Advanced Card Systems, Ltd ACR1281U") {
		t.Fatal("ACR1281U should match")
	}
	if !isReaderLsusb("ID 072f:2224 acs reader") {
		t.Fatal("072f VID should match")
	}
	if isReaderLsusb("ID 2500:0020 Ettus Research LLC USRP B210") {
		t.Fatal("USRP must not match")
	}
}
