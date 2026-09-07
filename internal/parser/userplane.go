package parser

import (
	"bytes"
	"context"
	"errors"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	UserPlaneMaxCaptureBytes int64 = 64 << 20
	userPlaneOutputMax             = 128 << 10
	userPlaneTimeout               = 3 * time.Second
)

// UserPlaneObservation is a count-only summary of an existing SGi capture.
// Packet contents and endpoint addresses are never returned.
type UserPlaneObservation struct {
	State             string    `json:"state"`
	Reason            string    `json:"reason"`
	CaptureSize       int64     `json:"capture_size_bytes,omitempty"`
	CaptureModified   time.Time `json:"capture_modified_at,omitempty"`
	IPPackets         int       `json:"ip_packets"`
	UEUplinkPackets   int       `json:"ue_uplink_packets"`
	UEDownlinkPackets int       `json:"ue_downlink_packets"`
	DNSQueries        int       `json:"dns_queries"`
	DNSResponses      int       `json:"dns_responses"`
	ScanComplete      bool      `json:"scan_complete"`
	Limitations       []string  `json:"limitations,omitempty"`
}

type userPlaneRunner func(context.Context, string, ...string) ([]byte, bool, error)

// InspectUserPlane performs a bounded read-only scan of the existing SGi pcap.
func InspectUserPlane(ctx context.Context, tsharkBin, path string) UserPlaneObservation {
	return inspectUserPlane(ctx, tsharkBin, path, runUserPlaneCommand)
}

func inspectUserPlane(ctx context.Context, tsharkBin, path string, run userPlaneRunner) UserPlaneObservation {
	o := UserPlaneObservation{
		State:  "not_collected",
		Reason: "capture_missing",
		Limitations: []string{
			"packet counts cover all UEs in the SGi capture and are not associated with one subscriber",
			"observed packets do not by themselves prove Internet or DNS reachability",
		},
	}
	st, err := os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			o.State = "unavailable"
			o.Reason = "capture_stat_failed"
		}
		return o
	}
	o.CaptureSize = st.Size()
	o.CaptureModified = st.ModTime()
	if st.Size() == 0 {
		o.Reason = "capture_empty"
		return o
	}
	if st.Size() > UserPlaneMaxCaptureBytes {
		o.State = "not_checked"
		o.Reason = "capture_too_large"
		o.Limitations = append(o.Limitations,
			"capture exceeds the synchronous diagnostic size limit")
		return o
	}

	ctx2, cancel := context.WithTimeout(ctx, userPlaneTimeout)
	defer cancel()
	args := []string{
		"-n",
		"-r", path,
		"-Y", "ip",
		"-T", "fields",
		"-E", "occurrence=f",
		"-e", "ip.src",
		"-e", "ip.dst",
		"-e", "dns.flags.response",
	}
	out, truncated, err := run(ctx2, tsharkBin, args...)
	if err != nil {
		switch {
		case errors.Is(ctx2.Err(), context.DeadlineExceeded):
			o.State = "unavailable"
			o.Reason = "tshark_timeout"
			return o
		case errors.Is(err, exec.ErrNotFound), errors.Is(err, os.ErrNotExist):
			o.State = "unavailable"
			o.Reason = "tshark_missing"
			return o
		}
	}

	ueSubnet := netip.MustParsePrefix("172.16.0.0/24")
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(strings.TrimRight(line, "\r"), "\t")
		for len(fields) < 3 {
			fields = append(fields, "")
		}
		src, srcErr := netip.ParseAddr(strings.TrimSpace(fields[0]))
		dst, dstErr := netip.ParseAddr(strings.TrimSpace(fields[1]))
		if srcErr != nil && dstErr != nil {
			continue
		}
		o.IPPackets++
		if srcErr == nil && ueSubnet.Contains(src) {
			o.UEUplinkPackets++
		}
		if dstErr == nil && ueSubnet.Contains(dst) {
			o.UEDownlinkPackets++
		}
		switch strings.TrimSpace(fields[2]) {
		case "0":
			o.DNSQueries++
		case "1":
			o.DNSResponses++
		}
	}
	// A live pcap can have an incomplete final record while tcpdump is still
	// writing it. Preserve already decoded positive evidence, but never treat a
	// non-zero tshark exit as a complete scan or as proof of absence.
	if err != nil {
		if o.IPPackets == 0 {
			o.State = "unavailable"
			o.Reason = "capture_decode_failed"
			return o
		}
		o.State = "observed"
		o.Reason = "partial_decode"
		o.ScanComplete = false
		o.Limitations = append(o.Limitations,
			"positive packet counts were decoded before tshark reported an incomplete or invalid record; counts are lower bounds")
		return o
	}
	o.ScanComplete = !truncated
	if o.IPPackets > 0 {
		o.State = "observed"
		o.Reason = "ip_packets_observed"
	} else if truncated {
		o.State = "unknown"
		o.Reason = "tshark_output_truncated"
	} else {
		o.State = "not_observed"
		o.Reason = "no_ip_packets_observed"
	}
	if truncated {
		o.Limitations = append(o.Limitations,
			"packet counts are lower bounds because tshark output was bounded")
	}
	return o
}

type userPlaneBoundedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (b *userPlaneBoundedBuffer) Write(p []byte) (int, error) {
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

func (b *userPlaneBoundedBuffer) result() ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...), b.truncated
}

func runUserPlaneCommand(ctx context.Context, name string, args ...string) ([]byte, bool, error) {
	var stdout, stderr userPlaneBoundedBuffer
	stdout.max = userPlaneOutputMax
	stderr.max = userPlaneOutputMax
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out, truncated := stdout.result()
	_, stderrTruncated := stderr.result()
	return out, truncated || stderrTruncated, err
}
