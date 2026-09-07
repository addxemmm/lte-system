package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/addxemmm/lte-system/internal/crack"
	"github.com/addxemmm/lte-system/internal/lte"
	"github.com/addxemmm/lte-system/internal/parser"
)

const connectivityDiagnosticTimeout = 5 * time.Second

type dnsDiagnostics struct {
	State                 string   `json:"state"`
	ConfiguredServer      string   `json:"configured_server,omitempty"`
	ConfigurationEvidence string   `json:"configuration_evidence"`
	QueriesObserved       int      `json:"queries_observed"`
	ResponsesObserved     int      `json:"responses_observed"`
	Limitations           []string `json:"limitations"`
}

// writeV1CHAPFailure preserves the cracking contract while distinguishing an
// absent handshake from missing, unavailable, oversized, or undecodable input.
func (s *Server) writeV1CHAPFailure(w http.ResponseWriter, r *http.Request) {
	cell := s.mgr.IsRunning()
	ctx, cancel := context.WithTimeout(r.Context(), connectivityDiagnosticTimeout)
	defer cancel()
	o := crack.ObserveCHAP(ctx, s.cfg, cell.StartedAt)
	data := map[string]any{"reason": o.Reason, "state": o.State, "scan_complete": o.ScanComplete,
		"capture": o.Capture}
	switch o.Reason {
	case "capture_missing", "capture_empty", "capture_no_packets", "capture_predates_current_session":
		writeV1(w, r, CodePrecondition, "S1AP capture not collected for the current session", data)
	case "tshark_missing", "tshark_timeout", "tshark_incompatible", "inspection_timeout":
		writeV1(w, r, CodeDependency, "CHAP inspection dependency unavailable", data)
	case "capture_incomplete":
		writeV1(w, r, CodeUnprocessable, "S1AP capture is incomplete or changed during inspection; CHAP absence is not established", data)
	case "capture_stat_failed", "capture_read_failed", "capture_not_regular", "capture_format_invalid",
		"capture_linktype_unsupported", "capture_decode_failed", "capture_too_large", "tshark_output_truncated", "inspection_cancelled":
		writeV1(w, r, CodeUnprocessable, "S1AP capture could not be conclusively inspected", data)
	case "no_s1ap_frames_observed":
		writeV1(w, r, CodePrecondition, "no decodable S1AP frames observed in capture", data)
	case "no_chap_frames_observed":
		writeV1(w, r, CodePrecondition, "no CHAP exchange observed; APN authentication may not require CHAP", data)
	case "pap_frames_observed":
		writeV1(w, r, CodePrecondition, "PAP protocol metadata observed instead of CHAP; no credentials inspected", data)
	case "chap_frames_observed":
		writeV1(w, r, CodeUnprocessable, "CHAP frames were observed but a complete handshake was not extractable", data)
	default:
		writeV1(w, r, CodeUnprocessable, "CHAP visibility is inconclusive", data)
	}
}

type connectivityDiagnostics struct {
	Cell          lte.Status                   `json:"cell"`
	LogWindow     parser.EvidenceWindow        `json:"log_window"`
	Registration  parser.RegistrationEvidence  `json:"registration"`
	PDN           parser.PDNEvidence           `json:"pdn"`
	NASVisibility parser.NASVisibilityEvidence `json:"nas_visibility"`
	CHAP          crack.CHAPObservation        `json:"chap"`
	UserPlane     parser.UserPlaneObservation  `json:"user_plane"`
	DNS           dnsDiagnostics               `json:"dns"`
	Network       lte.NetworkDiagnostics       `json:"network"`
	Limitations   []string                     `json:"limitations"`
}

// handleV1ConnectivityDiagnostics reads existing logs, captures and host
// configuration with bounded tshark readers. It never starts/stops the cell,
// creates a capture, sends probe packets, mutates rules or exposes credentials.
func (s *Server) handleV1ConnectivityDiagnostics(w http.ResponseWriter, r *http.Request) {
	select {
	case s.diagGate <- struct{}{}:
		defer func() { <-s.diagGate }()
	default:
		writeV1(w, r, CodeTooManyRequests, "connectivity diagnostic already running",
			map[string]any{"state": "busy", "reason": "diagnostic_busy"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), connectivityDiagnosticTimeout)
	defer cancel()

	cell := s.mgr.IsRunning()
	plan := s.mgr.NetworkPlanSnapshot()
	logEvidence := parser.InspectConnectivityLog(s.cfg.LogPath(s.cfg.EPCLogName))

	var wg sync.WaitGroup
	var chapEvidence crack.CHAPObservation
	var userPlane parser.UserPlaneObservation
	var network lte.NetworkDiagnostics
	wg.Add(3)
	go func() {
		defer wg.Done()
		chapEvidence = crack.ObserveCHAP(ctx, s.cfg, cell.StartedAt)
	}()
	go func() {
		defer wg.Done()
		ueSubnet := ""
		if plan.Active {
			ueSubnet = plan.UESubnet
		}
		userPlane = parser.InspectUserPlaneForSubnet(ctx, s.cfg.TsharkBin, s.cfg.LogPath(s.cfg.PcapLTEData), ueSubnet)
	}()
	go func() {
		defer wg.Done()
		network = s.mgr.NetworkDiagnostics(ctx)
	}()
	wg.Wait()

	if chapEvidence.State == "not_observed" && logEvidence.NASVisibility.CipheredMessages > 0 {
		chapEvidence.Limitations = append(chapEvidence.Limitations,
			"ciphered NAS messages were observed in the EPC log; this limits what an S1AP capture alone can expose")
	}
	dns := buildDNSDiagnostics(s.mgr, userPlane)
	data := connectivityDiagnostics{
		Cell:          cell,
		LogWindow:     logEvidence.Window,
		Registration:  logEvidence.Registration,
		PDN:           logEvidence.PDN,
		NASVisibility: logEvidence.NASVisibility,
		CHAP:          chapEvidence,
		UserPlane:     userPlane,
		DNS:           dns,
		Network:       network,
		Limitations: append(logEvidence.Limitations,
			"registration, bearer, DNS, user-plane, and host-network evidence are independent layers",
			"an allocated UE address or present firewall rule does not prove Internet reachability"),
	}
	writeV1(w, r, CodeOK, "connectivity evidence", data)
}

func buildDNSDiagnostics(mgr *lte.Manager, user parser.UserPlaneObservation) dnsDiagnostics {
	d := dnsDiagnostics{
		State:                 "unknown",
		ConfigurationEvidence: "not_available",
		QueriesObserved:       user.DNSQueries,
		ResponsesObserved:     user.DNSResponses,
		Limitations: []string{
			"the configured server is profile evidence, not proof that the UE received or reached it",
			"DNS counts cover the bounded aggregate SGi capture, not one UE",
		},
	}
	if p, ok := mgr.LoadProfile(); ok && p.DNS != "" {
		d.ConfiguredServer = p.DNS
		d.ConfigurationEvidence = "saved_start_profile"
	}
	switch {
	case user.DNSResponses > 0:
		d.State = "response_observed"
	case user.DNSQueries > 0 && user.ScanComplete:
		d.State = "query_without_response_observed"
	case user.DNSQueries > 0:
		d.State = "query_observed_in_partial_scan"
	case user.ScanComplete:
		d.State = "not_observed"
	}
	return d
}
