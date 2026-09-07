package crack

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/addxemmm/lte-system/internal/config"
)

func observationConfig(t *testing.T, capture []byte) config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.LogDir = t.TempDir()
	if capture != nil {
		if err := os.WriteFile(filepath.Join(cfg.LogDir, cfg.PcapS1AP), capture, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return cfg
}

func TestObserveCHAPMissingAndEmpty(t *testing.T) {
	missing := observationConfig(t, nil)
	o := observeCHAP(context.Background(), missing, nil, nil)
	if o.State != "not_collected" || o.Reason != "capture_missing" {
		t.Fatalf("unexpected missing observation: %+v", o)
	}

	empty := observationConfig(t, []byte{})
	o = observeCHAP(context.Background(), empty, nil, nil)
	if o.State != "not_collected" || o.Reason != "capture_empty" {
		t.Fatalf("unexpected empty observation: %+v", o)
	}
}

func TestObserveCHAPClassifiesToolAndDecodeFailures(t *testing.T) {
	cfg := observationConfig(t, []byte("pcap"))
	missingTool := func(context.Context, string, ...string) ([]byte, bool, error) {
		return nil, false, exec.ErrNotFound
	}
	o := observeCHAP(context.Background(), cfg, nil, missingTool)
	if o.State != "unavailable" || o.Reason != "tshark_missing" {
		t.Fatalf("unexpected missing-tool observation: %+v", o)
	}
	missingAbsolute := func(context.Context, string, ...string) ([]byte, bool, error) {
		return nil, false, &os.PathError{Op: "fork/exec", Path: "/missing/tshark", Err: os.ErrNotExist}
	}
	if got := observeCHAP(context.Background(), cfg, nil, missingAbsolute); got.Reason != "tshark_missing" {
		t.Fatalf("absolute missing executable must not be classified as bad capture: %+v", got)
	}

	decodeFailure := func(context.Context, string, ...string) ([]byte, bool, error) {
		return nil, false, errors.New("invalid capture")
	}
	o = observeCHAP(context.Background(), cfg, nil, decodeFailure)
	if o.State != "unavailable" || o.Reason != "capture_decode_failed" || o.Capture.State != "decode_failed" {
		t.Fatalf("unexpected decode observation: %+v", o)
	}
}

func TestObserveCHAPProtocolStates(t *testing.T) {
	cfg := observationConfig(t, []byte("pcap"))
	noCHAP := func(context.Context, string, ...string) ([]byte, bool, error) {
		return []byte("1\teth:s1ap\t\n2\teth:s1ap\t\n"), false, nil
	}
	o := observeCHAP(context.Background(), cfg, nil, noCHAP)
	if o.State != "not_observed" || o.Reason != "no_chap_frames_observed" ||
		!o.S1APObserved || o.CHAPObserved || !o.ScanComplete {
		t.Fatalf("unexpected no-CHAP observation: %+v", o)
	}

	withCHAP := func(context.Context, string, ...string) ([]byte, bool, error) {
		return []byte("1\teth:s1ap\t\n2\teth:s1ap:chap\t2\n"), false, nil
	}
	o = observeCHAP(context.Background(), cfg, nil, withCHAP)
	if o.State != "observed" || !o.CHAPObserved || !o.ScanComplete {
		t.Fatalf("unexpected CHAP observation: %+v", o)
	}

	truncated := func(context.Context, string, ...string) ([]byte, bool, error) {
		return []byte("1\teth:s1ap\t\n"), true, nil
	}
	o = observeCHAP(context.Background(), cfg, nil, truncated)
	if o.State != "unknown" || o.Reason != "tshark_output_truncated" || o.ScanComplete {
		t.Fatalf("truncated output must not prove absence: %+v", o)
	}
}

func TestObserveCHAPPredatesCurrentSession(t *testing.T) {
	cfg := observationConfig(t, []byte("pcap"))
	p := filepath.Join(cfg.LogDir, cfg.PcapS1AP)
	old := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	o := observeCHAP(context.Background(), cfg, &started, nil)
	if o.State != "not_collected" || o.Reason != "capture_predates_current_session" {
		t.Fatalf("unexpected stale observation: %+v", o)
	}
}
