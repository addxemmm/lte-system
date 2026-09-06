package sim

import "testing"

// TestReaderDetection pins the reported bug: with NO reader attached, the
// chain must report (false,false) -> message_id 2, never "card missing".
func TestReaderPresentLsusb(t *testing.T) {
	if !readerPresentLsusb("Bus 001 Device 002: ID 072f:2209 Advanced Card Systems, Ltd ACR1281U") {
		t.Fatal("ACR1281U line should match")
	}
	if !readerPresentLsusb("bus 003 device 005: ID 072f:2224 acs acr122u") {
		t.Fatal("072f VID should match (any ACS reader)")
	}
	if readerPresentLsusb("Bus 004 Device 002: ID 2500:0020 Ettus Research LLC USRP B210") {
		t.Fatal("USRP must not match")
	}
	if readerPresentLsusb("") {
		t.Fatal("empty must not match")
	}
}

func TestParsePcscScan(t *testing.T) {
	withCard := "Using reader plug'n play mechanism\n" +
		"Scanning present readers...\n" +
		"0: ACS ACR1281U 00 00\n\n" +
		" Reader 0: ACS ACR1281U 00 00\n" +
		"  Card state: Card inserted, \n"
	p, c := parsePcscScan(withCard)
	if !p || !c {
		t.Fatalf("want present+card: %v %v", p, c)
	}
	noCard := "Scanning present readers...\n" +
		"0: ACS ACR1281U 00 00\n\n" +
		" Reader 0: ACS ACR1281U 00 00\n" +
		"  Card state: Card removed, \n"
	p, c = parsePcscScan(noCard)
	if !p || c {
		t.Fatalf("want present, no card: %v %v", p, c)
	}
	waiting := "Using reader plug'n play mechanism\n" +
		"Scanning present readers...\n" +
		"Waiting for the first reader...\n"
	p, c = parsePcscScan(waiting)
	if p || c {
		t.Fatalf("spinner without readers must be absent: %v %v", p, c)
	}
}

func TestCardEvidence(t *testing.T) {
	// Python traceback must NEVER count as evidence (the id5-vs-id2 bug).
	trace := "Traceback (most recent call last):\n" +
		"  File \"/opt/pysim/pySim-read.py\", line 31, in <module>\n" +
		"ModuleNotFoundError: No module named 'osmocom'\n"
	if cardEvidence(trace) {
		t.Fatal("traceback must not count as card evidence")
	}
	if cardEvidence("") {
		t.Fatal("empty must not count")
	}
	good := "Reading ...\nATR: 3B 9F 95 80\nIMSI: 001010123456789\n"
	if !cardEvidence(good) {
		t.Fatal("genuine read output must count")
	}
}
