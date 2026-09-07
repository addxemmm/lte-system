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
	chapObservationWaitDelay             = 250 * time.Millisecond
)

// CaptureObservation reports metadata only; it never returns packet contents.
type CaptureObservation struct {
	State           string    `json:"state"`
	SizeBytes       int64     `json:"size_bytes,omitempty"`
	ModifiedAt      time.Time `json:"modified_at,omitempty"`
	SessionRelation string    `json:"session_relation,omitempty"`
	SnapshotSize    int64     `json:"snapshot_size_bytes,omitempty"`
	CompletePackets uint64    `json:"complete_packets,omitempty"`
	Incomplete      bool      `json:"incomplete,omitempty"`
	TailIncomplete  bool      `json:"tail_incomplete,omitempty"`
	SourceChanged   bool      `json:"source_changed,omitempty"`
	TrailingBytes   int64     `json:"trailing_bytes,omitempty"`
}

// CHAPObservation describes protocol visibility without returning a username,
// challenge, response, hash, or password.
type CHAPObservation struct {
	State        string             `json:"state"`
	Reason       string             `json:"reason"`
	Capture      CaptureObservation `json:"capture"`
	S1APObserved bool               `json:"s1ap_observed"`
	CHAPObserved bool               `json:"chap_observed"`
	PAPObserved  bool               `json:"pap_observed"`
	ScanComplete bool               `json:"scan_complete"`
	Limitations  []string           `json:"limitations,omitempty"`
}

type observationCommandResult struct {
	Output          []byte
	OutputTruncated bool
	Failure         string
	Err             error
}

type observationRunner func(context.Context, string, ...string) observationCommandResult

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
	ctx2, cancel := context.WithTimeout(ctx, chapObservationTimeout)
	defer cancel()
	snapshot, err := snapshotClassicPCAP(ctx2, path, CHAPObservationMaxCaptureBytes)
	predates := false
	if snapshot != nil && snapshot.sourceInfo != nil {
		// Always report metadata for the file descriptor that was actually
		// copied. The preliminary path Lstat is only a missing-file fast path
		// and may refer to an object replaced before Open.
		o.Capture.SizeBytes = snapshot.sourceInfo.Size()
		o.Capture.ModifiedAt = snapshot.sourceInfo.ModTime()
		o.Capture.SourceChanged = snapshot.sourceMutated
		o.Capture.State = "present"
		if startedAt != nil {
			// The capture is normally created shortly before Manager.startedAt
			// is recorded, hence the process-initialization tolerance.
			predates = snapshot.sourceInfo.ModTime().Before(startedAt.Add(-30 * time.Second))
			if predates {
				o.Capture.SessionRelation = "predates_current_session"
			} else {
				o.Capture.SessionRelation = "current_or_recent"
			}
		}
	}
	if err != nil {
		switch {
		case errors.Is(ctx2.Err(), context.DeadlineExceeded):
			o.State = "unavailable"
			o.Reason = "inspection_timeout"
		case errors.Is(ctx2.Err(), context.Canceled):
			o.State = "unavailable"
			o.Reason = "inspection_cancelled"
		case os.IsNotExist(err):
			o.State = "not_collected"
			o.Reason = "capture_missing"
			o.Capture.State = "missing"
		case errors.Is(err, errCaptureTooLarge):
			o.State = "not_checked"
			o.Reason = "capture_too_large"
			o.Limitations = append(o.Limitations,
				"capture exceeds the synchronous diagnostic size limit")
		case errors.Is(err, errCaptureIncomplete), errors.Is(err, errCaptureChanged):
			o.State = "unknown"
			o.Reason = "capture_incomplete"
			o.Capture.State = "incomplete"
			o.Capture.Incomplete = true
			o.Capture.TailIncomplete = errors.Is(err, errCaptureIncomplete)
			o.Capture.SourceChanged = o.Capture.SourceChanged || errors.Is(err, errCaptureChanged)
		case errors.Is(err, errCaptureFormat):
			o.State = "unavailable"
			o.Reason = "capture_format_invalid"
			o.Capture.State = "invalid"
		case errors.Is(err, errCaptureLinkType):
			o.State = "unavailable"
			o.Reason = "capture_linktype_unsupported"
			o.Capture.State = "invalid"
		case errors.Is(err, errCaptureNotRegular):
			o.State = "unavailable"
			o.Reason = "capture_not_regular"
			o.Capture.State = "unreadable"
		default:
			o.State = "unavailable"
			o.Reason = "capture_read_failed"
			o.Capture.State = "unreadable"
		}
		return o
	}
	if snapshot.path != "" {
		defer snapshot.cleanup()
	}
	o.Capture.SnapshotSize = snapshot.sizeBytes
	o.Capture.CompletePackets = snapshot.completePackets
	o.Capture.Incomplete = snapshot.incomplete
	o.Capture.TailIncomplete = snapshot.tailIncomplete
	o.Capture.SourceChanged = snapshot.sourceMutated
	o.Capture.TrailingBytes = snapshot.trailingBytes
	if snapshot.sourceChanged(path) {
		snapshot.incomplete = true
		o.Capture.Incomplete = true
		o.Capture.SourceChanged = true
	}
	if snapshot.incomplete {
		o.Capture.State = "incomplete"
		o.Limitations = append(o.Limitations,
			"the snapshot contains only complete pcap records; framing completeness does not establish successful decoding")
	}
	if predates {
		if snapshot.incomplete {
			o.State = "unknown"
			o.Reason = "capture_incomplete"
		} else {
			o.Reason = "capture_predates_current_session"
		}
		return o
	}
	if snapshot.sourceInfo.Size() == 0 {
		if snapshot.incomplete {
			o.State = "unknown"
			o.Reason = "capture_incomplete"
		} else {
			o.Reason = "capture_empty"
		}
		return o
	}
	if snapshot.completePackets == 0 {
		if snapshot.incomplete {
			o.State = "unknown"
			o.Reason = "capture_incomplete"
		} else {
			o.Reason = "capture_no_packets"
		}
		return o
	}

	args := []string{
		"-n",
		"-o", `uat:user_dlts:"User 3 (DLT=150)","s1ap","0","","0",""`,
		"-r", snapshot.path,
		"-Y", "s1ap || chap || pap",
		"-T", "fields",
		"-E", "occurrence=f",
		"-e", "frame.number",
		"-e", "frame.protocols",
		"-e", "chap.code",
	}
	result := run(ctx2, cfg.TsharkBin, args...)
	if snapshot.sourceChanged(path) {
		snapshot.incomplete = true
		o.Capture.Incomplete = true
		o.Capture.SourceChanged = true
		o.Capture.State = "incomplete"
		o.Limitations = append(o.Limitations,
			"the source capture changed while its immutable prefix was inspected")
	}
	if errors.Is(ctx2.Err(), context.DeadlineExceeded) {
		o.State = "unavailable"
		o.Reason = "tshark_timeout"
		return o
	}
	if errors.Is(ctx2.Err(), context.Canceled) {
		o.State = "unavailable"
		o.Reason = "inspection_cancelled"
		return o
	}

	for _, line := range strings.Split(strings.TrimSpace(string(result.Output)), "\n") {
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
		if strings.Contains(protocols, "pap") {
			o.PAPObserved = true
		}
	}

	if result.Err != nil {
		switch {
		case errors.Is(ctx2.Err(), context.DeadlineExceeded):
			o.State = "unavailable"
			o.Reason = "tshark_timeout"
		case errors.Is(ctx2.Err(), context.Canceled):
			o.State = "unavailable"
			o.Reason = "inspection_cancelled"
		case errors.Is(result.Err, exec.ErrNotFound), errors.Is(result.Err, os.ErrNotExist):
			o.State = "unavailable"
			o.Reason = "tshark_missing"
		case result.Failure == "capture_incomplete" && o.CHAPObserved:
			o.State = "observed"
			o.Reason = "chap_frames_observed"
			o.Capture.State = "incomplete"
			o.Capture.Incomplete = true
			o.Limitations = append(o.Limitations,
				"CHAP metadata was decoded only from a complete prefix")
		case result.Failure == "capture_incomplete":
			o.State = "unknown"
			o.Reason = "capture_incomplete"
			o.Capture.State = "incomplete"
			o.Capture.Incomplete = true
		default:
			o.State = "unavailable"
			o.Reason = result.Failure
			if o.Reason == "" {
				o.Reason = "capture_decode_failed"
			}
			o.Capture.State = "decode_failed"
		}
		return o
	}
	if o.CHAPObserved {
		o.State = "observed"
		o.Reason = "chap_frames_observed"
		o.ScanComplete = !result.OutputTruncated && !snapshot.incomplete
		if result.OutputTruncated {
			o.Limitations = append(o.Limitations, "tshark output was bounded")
		}
		return o
	}
	if result.OutputTruncated {
		o.State = "unknown"
		o.Reason = "tshark_output_truncated"
		o.Limitations = append(o.Limitations,
			"bounded tshark output cannot prove CHAP was absent later in the capture")
		return o
	}
	if snapshot.incomplete {
		o.State = "unknown"
		o.Reason = "capture_incomplete"
		o.Limitations = append(o.Limitations,
			"absence in a prefix does not prove that later packets contain no CHAP exchange")
		return o
	}
	o.ScanComplete = true
	o.State = "not_observed"
	if o.PAPObserved {
		o.Reason = "pap_frames_observed"
		o.Limitations = append(o.Limitations,
			"PAP protocol metadata was observed; no PAP identity or password was read")
	} else if o.S1APObserved {
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

func runObservationCommand(ctx context.Context, name string, args ...string) observationCommandResult {
	var stdout, stderr boundedBuffer
	stdout.max = chapObservationOutputMax
	stderr.max = chapObservationOutputMax
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Bound Wait when a failed decoder leaves descendants holding inherited
	// stdout/stderr pipes. CommandContext still owns the process cancellation;
	// WaitDelay only prevents pipe EOF from extending it indefinitely.
	cmd.WaitDelay = chapObservationWaitDelay
	err := cmd.Run()
	out, truncated := stdout.result()
	errOut, stderrTruncated := stderr.result()
	failure := classifyTsharkFailure(errOut)
	if errors.Is(err, exec.ErrWaitDelay) {
		failure = "tshark_wait_timeout"
	}
	return observationCommandResult{
		Output:          out,
		OutputTruncated: truncated || stderrTruncated,
		Failure:         failure,
		Err:             err,
	}
}

// classifyTsharkFailure converts bounded stderr into stable metadata. Raw
// stderr is intentionally never exposed because dissector diagnostics can
// contain paths or packet-rendered text.
func classifyTsharkFailure(stderr []byte) string {
	s := strings.ToLower(string(stderr))
	switch {
	case strings.Contains(s, "cut short"), strings.Contains(s, "middle of a packet"):
		return "capture_incomplete"
	case strings.Contains(s, "permission denied"), strings.Contains(s, "could not open"):
		return "capture_read_failed"
	case strings.Contains(s, "not a capture file"), strings.Contains(s, "isn't a capture file"),
		strings.Contains(s, "appears to be damaged"), strings.Contains(s, "corrupt"):
		return "capture_format_invalid"
	case strings.Contains(s, "some fields aren't valid"), strings.Contains(s, "isn't a valid field"),
		strings.Contains(s, "invalid -o"), strings.Contains(s, "unknown preference"),
		strings.Contains(s, "syntax error"):
		return "tshark_incompatible"
	default:
		return "capture_decode_failed"
	}
}
