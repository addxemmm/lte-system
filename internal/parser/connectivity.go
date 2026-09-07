package parser

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// DiagnosticLogMaxBytes bounds the tail window used by the read-only
// connectivity diagnostic. Runtime EPC logs can grow without limit.
const DiagnosticLogMaxBytes int64 = 2 << 20

// EvidenceWindow describes exactly how much of the EPC log was inspected.
// It deliberately contains no path or log text.
type EvidenceWindow struct {
	State      string    `json:"state"`
	FileSize   int64     `json:"file_size_bytes,omitempty"`
	BytesRead  int64     `json:"bytes_read,omitempty"`
	Truncated  bool      `json:"truncated"`
	ModifiedAt time.Time `json:"modified_at,omitempty"`
	Problem    string    `json:"problem,omitempty"`
}

// RegistrationEvidence reports independent, aggregate observations. Events
// are not joined to an IMSI because srsEPC log lines do not consistently carry
// a stable S1AP identifier and multiple UEs can interleave.
type RegistrationEvidence struct {
	State                  string `json:"state"`
	Scope                  string `json:"scope"`
	S1APIDsObserved        int    `json:"s1ap_ids_observed"`
	AttachRequests         int    `json:"attach_requests"`
	AuthenticationAccepted int    `json:"authentication_accepted"`
	SecurityModeComplete   int    `json:"security_mode_complete"`
	AttachComplete         int    `json:"attach_complete"`
	Rejects                int    `json:"rejects"`
}

// PDNEvidence reports bearer-stage observations without joining APN, address,
// or subscriber values from unrelated log lines.
type PDNEvidence struct {
	State             string `json:"state"`
	Scope             string `json:"scope"`
	Requests          int    `json:"requests"`
	IPAllocations     int    `json:"ip_allocations"`
	BearerActivations int    `json:"bearer_activations"`
	Rejects           int    `json:"rejects"`
}

// NASVisibilityEvidence only describes whether ciphering markers were seen in
// the bounded log window. It does not claim that a particular CHAP frame was
// encrypted.
type NASVisibilityEvidence struct {
	CipheredMessages int `json:"ciphered_messages"`
	PlainMessages    int `json:"plain_messages"`
}

// ConnectivityEvidence is a bounded, credential-free summary of EPC logs.
type ConnectivityEvidence struct {
	Window        EvidenceWindow        `json:"window"`
	Registration  RegistrationEvidence  `json:"registration"`
	PDN           PDNEvidence           `json:"pdn"`
	NASVisibility NASVisibilityEvidence `json:"nas_visibility"`
	Limitations   []string              `json:"limitations"`
}

var enbUES1APID = regexp.MustCompile(`(?i)eNB-UE S1AP Id:\s*([0-9a-fx]+)`)

// InspectConnectivityLog reads at most DiagnosticLogMaxBytes from the end of
// path. Missing/unreadable logs are diagnostic states, not API failures.
func InspectConnectivityLog(path string) ConnectivityEvidence {
	d := ConnectivityEvidence{
		Window:       EvidenceWindow{State: "not_collected"},
		Registration: RegistrationEvidence{State: "not_observed", Scope: "aggregate_only"},
		PDN:          PDNEvidence{State: "not_observed", Scope: "aggregate_only"},
		Limitations: []string{
			"evidence is aggregate-only because EPC log lines cannot always be associated with one UE",
			"absence from the inspected log window is not proof that an event never occurred",
		},
	}

	f, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			d.Window.State = "unavailable"
			d.Window.Problem = "epc_log_unreadable"
		}
		return d
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		d.Window.State = "unavailable"
		d.Window.Problem = "epc_log_stat_failed"
		return d
	}
	d.Window.FileSize = st.Size()
	d.Window.ModifiedAt = st.ModTime()
	if st.Size() == 0 {
		return d
	}

	start := int64(0)
	if st.Size() > DiagnosticLogMaxBytes {
		start = st.Size() - DiagnosticLogMaxBytes
		d.Window.Truncated = true
		d.Limitations = append(d.Limitations,
			"only the bounded tail of the EPC log was inspected")
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		d.Window.State = "unavailable"
		d.Window.Problem = "epc_log_seek_failed"
		return d
	}
	b, err := io.ReadAll(io.LimitReader(f, DiagnosticLogMaxBytes))
	if err != nil {
		d.Window.State = "unavailable"
		d.Window.Problem = "epc_log_read_failed"
		return d
	}
	d.Window.BytesRead = int64(len(b))
	d.Window.State = "present"
	// A tail seek can begin in the middle of a line; exclude that partial line
	// rather than interpreting fragments as evidence.
	if start > 0 {
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		} else {
			b = nil
		}
	}

	ids := make(map[string]struct{})
	var attachInitial, attachWithID, attachWithIMSI int
	for _, raw := range bytes.Split(b, []byte{'\n'}) {
		line := string(raw)
		low := strings.ToLower(line)
		if strings.Contains(low, "attach request") {
			if strings.Contains(low, "received initial ue message") {
				attachInitial++
			}
			if strings.Contains(low, "-- imsi:") {
				attachWithIMSI++
			}
			if m := enbUES1APID.FindStringSubmatch(line); m != nil {
				attachWithID++
				ids[strings.ToLower(m[1])] = struct{}{}
			}
		}
		switch {
		case strings.Contains(low, "ue authentication accepted"):
			d.Registration.AuthenticationAccepted++
		case strings.Contains(low, "security mode command complete") ||
			strings.Contains(low, "received security mode complete"):
			d.Registration.SecurityModeComplete++
		case strings.Contains(low, "received attach complete"):
			d.Registration.AttachComplete++
		}
		if strings.Contains(low, "authentication rejected") ||
			strings.Contains(low, "attach reject") || strings.Contains(low, "attach rejected") {
			d.Registration.Rejects++
		}
		if strings.Contains(low, "pdn connectivity request") {
			d.PDN.Requests++
		}
		if strings.Contains(low, "pool ip addr") || strings.Contains(low, "static ip addr") ||
			strings.Contains(low, "init_ue_ip") {
			d.PDN.IPAllocations++
		}
		if strings.Contains(low, "activated eps bearer") {
			d.PDN.BearerActivations++
		}
		if strings.Contains(low, "pdn connectivity reject") || strings.Contains(low, "pdn connectivity rejected") {
			d.PDN.Rejects++
		}
		if strings.Contains(low, "msg_encrypted: yes") {
			d.NASVisibility.CipheredMessages++
		} else if strings.Contains(low, "msg_encrypted: no") {
			d.NASVisibility.PlainMessages++
		}
	}
	d.Registration.S1APIDsObserved = len(ids)
	d.Registration.AttachRequests = maxInt(attachInitial, maxInt(attachWithID, attachWithIMSI))
	d.Registration.State = registrationState(d.Registration, d.Window.Truncated)
	d.PDN.State = pdnState(d.PDN, d.Window.Truncated)
	if len(ids) > 1 {
		d.Limitations = append(d.Limitations,
			"multiple S1AP UE identifiers were observed; independent events are not combined into a per-UE result")
	}
	return d
}

func registrationState(e RegistrationEvidence, truncated bool) string {
	positive := e.AttachComplete > 0 || e.SecurityModeComplete > 0 ||
		e.AuthenticationAccepted > 0 || e.AttachRequests > 0
	if e.Rejects > 0 && positive {
		return "mixed_observations"
	}
	if e.AttachComplete > 0 {
		return "attach_complete_observed"
	}
	if e.Rejects > 0 {
		return "rejected_observed"
	}
	if e.SecurityModeComplete > 0 {
		return "security_mode_complete_observed"
	}
	if e.AuthenticationAccepted > 0 {
		return "authentication_accepted_observed"
	}
	if e.AttachRequests > 0 {
		return "attach_requested_observed"
	}
	if truncated {
		return "unknown_truncated_window"
	}
	return "not_observed"
}

func pdnState(e PDNEvidence, truncated bool) string {
	positive := e.BearerActivations > 0 || e.IPAllocations > 0 || e.Requests > 0
	if e.Rejects > 0 && positive {
		return "mixed_observations"
	}
	if e.BearerActivations > 0 {
		return "bearer_activation_observed"
	}
	if e.Rejects > 0 {
		return "rejected_observed"
	}
	if e.IPAllocations > 0 {
		return "ip_allocation_observed"
	}
	if e.Requests > 0 {
		return "connectivity_requested_observed"
	}
	if truncated {
		return "unknown_truncated_window"
	}
	return "not_observed"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
