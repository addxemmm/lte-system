package parser

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func userPlaneCapture(t *testing.T, body []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "lte_data.pcap")
	if body != nil {
		if err := os.WriteFile(p, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestInspectUserPlaneCountsDirectionsAndDNS(t *testing.T) {
	p := userPlaneCapture(t, []byte("pcap"))
	run := func(context.Context, string, ...string) ([]byte, bool, error) {
		return []byte(
			"172.16.0.2\t8.8.8.8\t0\n" +
				"8.8.8.8\t172.16.0.2\t1\n" +
				"172.16.0.3\t1.1.1.1\t\n"), false, nil
	}
	o := inspectUserPlane(context.Background(), "tshark", p, run)
	if o.State != "observed" || o.IPPackets != 3 || o.UEUplinkPackets != 2 ||
		o.UEDownlinkPackets != 1 || o.DNSQueries != 1 || o.DNSResponses != 1 || !o.ScanComplete {
		t.Fatalf("unexpected observation: %+v", o)
	}
}

func TestInspectUserPlaneUsesEffectiveSubnetWithoutDefaultFallback(t *testing.T) {
	p := userPlaneCapture(t, []byte("pcap"))
	run := func(context.Context, string, ...string) ([]byte, bool, error) {
		return []byte("10.23.4.2\t8.8.8.8\t0\n8.8.8.8\t10.23.4.2\t1\n172.16.0.2\t1.1.1.1\t\n"), false, nil
	}
	o := inspectUserPlaneForSubnet(context.Background(), "tshark", p, "10.23.4.0/24", run)
	if o.ClassificationSubnet != "10.23.4.0/24" || o.UEUplinkPackets != 1 || o.UEDownlinkPackets != 1 {
		t.Fatalf("did not use effective subnet: %+v", o)
	}
	o = inspectUserPlaneForSubnet(context.Background(), "tshark", p, "", run)
	if o.UEUplinkPackets != 0 || o.UEDownlinkPackets != 0 || o.IPPackets != 3 || len(o.Limitations) < 3 {
		t.Fatalf("missing plan silently fell back or lost aggregate evidence: %+v", o)
	}
}

func TestInspectUserPlaneDoesNotClaimAbsenceWhenTruncated(t *testing.T) {
	p := userPlaneCapture(t, []byte("pcap"))
	run := func(context.Context, string, ...string) ([]byte, bool, error) {
		return nil, true, nil
	}
	o := inspectUserPlane(context.Background(), "tshark", p, run)
	if o.State != "unknown" || o.Reason != "tshark_output_truncated" || o.ScanComplete {
		t.Fatalf("unexpected truncated state: %+v", o)
	}
}

func TestInspectUserPlaneClassifiesFailures(t *testing.T) {
	p := userPlaneCapture(t, []byte("pcap"))
	missing := func(context.Context, string, ...string) ([]byte, bool, error) {
		return nil, false, exec.ErrNotFound
	}
	o := inspectUserPlane(context.Background(), "tshark", p, missing)
	if o.State != "unavailable" || o.Reason != "tshark_missing" {
		t.Fatalf("unexpected missing tool: %+v", o)
	}
	missingAbsolute := func(context.Context, string, ...string) ([]byte, bool, error) {
		return nil, false, &os.PathError{Op: "fork/exec", Path: "/missing/tshark", Err: os.ErrNotExist}
	}
	if got := inspectUserPlane(context.Background(), "tshark", p, missingAbsolute); got.Reason != "tshark_missing" {
		t.Fatalf("absolute missing executable must not be classified as bad capture: %+v", got)
	}
	bad := func(context.Context, string, ...string) ([]byte, bool, error) {
		return nil, false, errors.New("bad pcap")
	}
	o = inspectUserPlane(context.Background(), "tshark", p, bad)
	if o.State != "unavailable" || o.Reason != "capture_decode_failed" {
		t.Fatalf("unexpected decode failure: %+v", o)
	}
}

func TestInspectUserPlanePreservesPositiveEvidenceFromLivePartialCapture(t *testing.T) {
	p := userPlaneCapture(t, []byte("pcap being written"))
	run := func(context.Context, string, ...string) ([]byte, bool, error) {
		return []byte("172.16.0.2\t8.8.8.8\t0\n8.8.8.8\t172.16.0.2\t1\n"), false,
			errors.New("tshark: final record is incomplete")
	}
	o := inspectUserPlane(context.Background(), "tshark", p, run)
	if o.State != "observed" || o.Reason != "partial_decode" || o.ScanComplete ||
		o.IPPackets != 2 || o.UEUplinkPackets != 1 || o.UEDownlinkPackets != 1 ||
		o.DNSQueries != 1 || o.DNSResponses != 1 {
		t.Fatalf("positive partial evidence was discarded or overstated: %+v", o)
	}
}

func TestInspectUserPlaneMissingAndEmpty(t *testing.T) {
	o := inspectUserPlane(context.Background(), "tshark", userPlaneCapture(t, nil), nil)
	if o.State != "not_collected" || o.Reason != "capture_missing" {
		t.Fatalf("unexpected missing state: %+v", o)
	}
	o = inspectUserPlane(context.Background(), "tshark", userPlaneCapture(t, []byte{}), nil)
	if o.State != "not_collected" || o.Reason != "capture_empty" {
		t.Fatalf("unexpected empty state: %+v", o)
	}
}
