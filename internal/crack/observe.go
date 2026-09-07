package crack

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/addxemmm/lte-system/internal/config"
)

const (
	// CHAPObservationMaxCaptureBytes avoids an unbounded scan of a long-running
	// live capture from a synchronous HTTP request.
	CHAPObservationMaxCaptureBytes int64 = 64 << 20
	chapObservationOutputMax             = 128 << 10
	chapObservationTimeout               = 3 * time.Second
)

// CaptureObservation reports metadata only; it never returns packet contents.
type CaptureObservation struct {
	State           string    `json:"state"`
	SizeBytes       int64     `json:"size_bytes,omitempty"`
	ModifiedAt      time.Time `json:"modified_at,omitempty"`
	SessionRelation string    `json:"session_relation,omitempty"`
}

// CHAPObservation describes protocol visibility without returning a username,
// challenge, response, hash, or password.
type CHAPObservation struct {
	State        string             `json:"state"`
	Reason       string             `json:"reason"`
	Capture      CaptureObservation `json:"capture"`
	S1APObserved bool               `json:"s1ap_observed"`
	CHAPObserved bool               `json:"chap_observed"`
	ScanComplete bool               `json:"scan_complete"`
	Limitations  []string           `json:"limitations,omitempty"`
}

type observationRunner func(context.Context, string, ...string) ([]byte, bool, error)

// ObserveCHAP performs a bounded, read-only inspection of the existing S1AP
// capture. It is intentionally independent from ExtractCHAP and does not make
// password extraction or cracking more capable.
func ObserveCHAP(ctx context.Context, cfg config.Config, startedAt *time.Time) CHAPObservation {
	return observeCHAP(ctx, cfg, startedAt, runObservationCommand)
}

func observeCHAP(ctx context.Context, cfg config.Config, startedAt *time.Time, run observationRunner) CHAPObservation {
	o := CHAPObservation{
		State:   "not_collected",
		Reason:  "capture_missing",
		Capture: CaptureObservation{State: "missing"},
		Limitations: []string{
			"CHAP is optional APN authentication evidence, not LTE attach authentication",
			"absence of CHAP does not indicate an attach or Internet-connectivity failure",
		},
	}
	path := cfg.LogPath(cfg.PcapS1AP)
	st, err := os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			o.State = "unavailable"
			o.Reason = "capture_stat_failed"
			o.Capture.State = "unreadable"
		}
		return o
	}
	o.Capture.SizeBytes = st.Size()
	o.Capture.ModifiedAt = st.ModTime()
	o.Capture.State = "present"
	if startedAt != nil {
		// The capture is normally created shortly before Manager.startedAt is
		// recorded, hence the tolerance for process initialization.
		if st.ModTime().Before(startedAt.Add(-30 * time.Second)) {
			o.Capture.SessionRelation = "predates_current_session"
			o.Reason = "capture_predates_current_session"
			return o
		}
		o.Capture.SessionRelation = "current_or_recent"
	}
	if st.Size() == 0 {
		o.Reason = "capture_empty"
		return o
	}
	if st.Size() > CHAPObservationMaxCaptureBytes {
		o.State = "not_checked"
		o.Reason = "capture_too_large"
		o.Limitations = append(o.Limitations,
			"capture exceeds the synchronous diagnostic size limit")
		return o
	}

	ctx2, cancel := context.WithTimeout(ctx, chapObservationTimeout)
	defer cancel()
	args := []string{
		"-n",
		"-o", `uat:user_dlts:"User 3 (DLT=150)","s1ap","0","","0",""`,
		"-r", path,
		"-Y", "s1ap || chap",
		"-T", "fields",
		"-E", "occurrence=f",
		"-e", "frame.number",
		"-e", "frame.protocols",
		"-e", "chap.code",
	}
	out, outputTruncated, err := run(ctx2, cfg.TsharkBin, args...)
	if err != nil {
		switch {
		case errors.Is(ctx2.Err(), context.DeadlineExceeded):
			o.State = "unavailable"
			o.Reason = "tshark_timeout"
		case errors.Is(err, exec.ErrNotFound), errors.Is(err, os.ErrNotExist):
			o.State = "unavailable"
			o.Reason = "tshark_missing"
		default:
			o.State = "unavailable"
			o.Reason = "capture_decode_failed"
			o.Capture.State = "decode_failed"
		}
		return o
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(strings.TrimSpace(line), "\t")
		protocols := ""
		if len(fields) > 1 {
			protocols = strings.ToLower(strings.TrimSpace(fields[1]))
		}
		if strings.Contains(protocols, "s1ap") {
			o.S1APObserved = true
		}
		if strings.Contains(protocols, "chap") || (len(fields) > 2 && strings.TrimSpace(fields[2]) != "") {
			o.CHAPObserved = true
		}
	}
	if o.CHAPObserved {
		o.State = "observed"
		o.Reason = "chap_frames_observed"
		o.ScanComplete = !outputTruncated
		if outputTruncated {
			o.Limitations = append(o.Limitations, "tshark output was bounded")
		}
		return o
	}
	if outputTruncated {
		o.State = "unknown"
		o.Reason = "tshark_output_truncated"
		o.Limitations = append(o.Limitations,
			"bounded tshark output cannot prove CHAP was absent later in the capture")
		return o
	}
	o.ScanComplete = true
	o.State = "not_observed"
	if o.S1APObserved {
		o.Reason = "no_chap_frames_observed"
	} else {
		o.Reason = "no_s1ap_frames_observed"
	}
	return o
}

// boundedBuffer accepts all writes so the subprocess is not interrupted, but
// retains only a fixed prefix and records truncation. Stdout/stderr may be
// copied concurrently by os/exec.
type boundedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.max - b.buf.Len()
	if remaining > 0 {
		keep := len(p)
		if keep > remaining {
			keep = remaining
		}
		_, _ = b.buf.Write(p[:keep])
	}
	if len(p) > remaining {
		b.truncated = true
	}
	return len(p), nil
}

func (b *boundedBuffer) result() ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...), b.truncated
}

func runObservationCommand(ctx context.Context, name string, args ...string) ([]byte, bool, error) {
	var stdout, stderr boundedBuffer
	stdout.max = chapObservationOutputMax
	stderr.max = chapObservationOutputMax
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out, truncated := stdout.result()
	_, stderrTruncated := stderr.result()
	return out, truncated || stderrTruncated, err
}
