package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectConnectivityLogStages(t *testing.T) {
	p := filepath.Join(t.TempDir(), "epc.log")
	body := strings.Join([]string{
		"Received Initial UE message -- Attach Request",
		"Attach request -- eNB-UE S1AP Id: 1",
		"UE Authentication Accepted.",
		"Security Mode Command Complete -- IMSI: REDACTED",
		"PDN Connectivity Request -- Procedure Transaction Id: 4",
		"SPGW: get_new_ue_ipv4 pool ip addr 172.16.0.2",
		"UL NAS: sec_hdr_type: 2, msg_encrypted: yes",
		"UL NAS: Received Attach Complete",
		"Activated EPS Bearer: Bearer id 5",
	}, "\n")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	d := InspectConnectivityLog(p)
	if d.Window.State != "present" || d.Window.Truncated {
		t.Fatalf("unexpected window: %+v", d.Window)
	}
	if d.Registration.State != "attach_complete_observed" || d.Registration.AttachRequests != 1 ||
		d.Registration.AuthenticationAccepted != 1 || d.Registration.SecurityModeComplete != 1 ||
		d.Registration.AttachComplete != 1 {
		t.Fatalf("unexpected registration: %+v", d.Registration)
	}
	if d.PDN.State != "bearer_activation_observed" || d.PDN.Requests != 1 ||
		d.PDN.IPAllocations != 1 || d.PDN.BearerActivations != 1 {
		t.Fatalf("unexpected PDN evidence: %+v", d.PDN)
	}
	if d.NASVisibility.CipheredMessages != 1 {
		t.Fatalf("unexpected NAS evidence: %+v", d.NASVisibility)
	}
}

func TestInspectConnectivityLogDoesNotJoinMultipleUEs(t *testing.T) {
	p := filepath.Join(t.TempDir(), "epc.log")
	body := "Attach request -- eNB-UE S1AP Id: 1\n" +
		"Attach request -- eNB-UE S1AP Id: 2\n" +
		"UE Authentication Rejected.\n" +
		"UL NAS: Received Attach Complete\n" +
		"PDN Connectivity Reject\n" +
		"Activated EPS Bearer: Bearer id 5\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	d := InspectConnectivityLog(p)
	if d.Registration.Scope != "aggregate_only" || d.Registration.S1APIDsObserved != 2 ||
		d.Registration.State != "mixed_observations" {
		t.Fatalf("must remain aggregate and mixed: %+v", d.Registration)
	}
	if d.PDN.State != "mixed_observations" {
		t.Fatalf("must not combine reject and activation: %+v", d.PDN)
	}
	joined := strings.Join(d.Limitations, " ")
	if !strings.Contains(joined, "not combined") {
		t.Fatalf("missing multi-UE limitation: %v", d.Limitations)
	}
}

func TestInspectConnectivityLogBoundedTail(t *testing.T) {
	p := filepath.Join(t.TempDir(), "epc.log")
	prefix := strings.Repeat("x", int(DiagnosticLogMaxBytes)+128)
	if err := os.WriteFile(p, []byte(prefix+"\nno protocol evidence\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	d := InspectConnectivityLog(p)
	if !d.Window.Truncated || d.Window.BytesRead > DiagnosticLogMaxBytes {
		t.Fatalf("read was not bounded: %+v", d.Window)
	}
	if d.Registration.State != "unknown_truncated_window" || d.PDN.State != "unknown_truncated_window" {
		t.Fatalf("absence in a truncated window must be unknown: %+v %+v", d.Registration, d.PDN)
	}
}

func TestInspectConnectivityLogMissingAndEmpty(t *testing.T) {
	d := InspectConnectivityLog(filepath.Join(t.TempDir(), "missing.log"))
	if d.Window.State != "not_collected" || d.Registration.State != "not_observed" {
		t.Fatalf("unexpected missing state: %+v", d)
	}
	p := filepath.Join(t.TempDir(), "empty.log")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	d = InspectConnectivityLog(p)
	if d.Window.State != "not_collected" || d.Window.FileSize != 0 {
		t.Fatalf("unexpected empty state: %+v", d.Window)
	}
}
