package crack

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	cfg := observationConfig(t, syntheticS1APPCAP([]byte{1, 2, 3}))
	missingTool := func(context.Context, string, ...string) observationCommandResult {
		return observationCommandResult{Err: exec.ErrNotFound}
	}
	o := observeCHAP(context.Background(), cfg, nil, missingTool)
	if o.State != "unavailable" || o.Reason != "tshark_missing" {
		t.Fatalf("unexpected missing-tool observation: %+v", o)
	}
	missingAbsolute := func(context.Context, string, ...string) observationCommandResult {
		return observationCommandResult{Err: &os.PathError{Op: "fork/exec", Path: "/missing/tshark", Err: os.ErrNotExist}}
	}
	if got := observeCHAP(context.Background(), cfg, nil, missingAbsolute); got.Reason != "tshark_missing" {
		t.Fatalf("absolute missing executable must not be classified as bad capture: %+v", got)
	}

	decodeFailure := func(context.Context, string, ...string) observationCommandResult {
		return observationCommandResult{Failure: "capture_decode_failed", Err: errors.New("invalid capture")}
	}
	o = observeCHAP(context.Background(), cfg, nil, decodeFailure)
	if o.State != "unavailable" || o.Reason != "capture_decode_failed" || o.Capture.State != "decode_failed" {
		t.Fatalf("unexpected decode observation: %+v", o)
	}
}

func TestObserveCHAPProtocolStates(t *testing.T) {
	cfg := observationConfig(t, syntheticS1APPCAP([]byte{1, 2, 3}))
	noCHAP := func(context.Context, string, ...string) observationCommandResult {
		return observationCommandResult{Output: []byte("1\teth:s1ap\t\n2\teth:s1ap\t\n")}
	}
	o := observeCHAP(context.Background(), cfg, nil, noCHAP)
	if o.State != "not_observed" || o.Reason != "no_chap_frames_observed" ||
		!o.S1APObserved || o.CHAPObserved || !o.ScanComplete {
		t.Fatalf("unexpected no-CHAP observation: %+v", o)
	}

	withCHAP := func(context.Context, string, ...string) observationCommandResult {
		return observationCommandResult{Output: []byte("1\teth:s1ap\t\n2\teth:s1ap:chap\t2\n")}
	}
	o = observeCHAP(context.Background(), cfg, nil, withCHAP)
	if o.State != "observed" || !o.CHAPObserved || !o.ScanComplete {
		t.Fatalf("unexpected CHAP observation: %+v", o)
	}

	truncated := func(context.Context, string, ...string) observationCommandResult {
		return observationCommandResult{Output: []byte("1\teth:s1ap\t\n"), OutputTruncated: true}
	}
	o = observeCHAP(context.Background(), cfg, nil, truncated)
	if o.State != "unknown" || o.Reason != "tshark_output_truncated" || o.ScanComplete {
		t.Fatalf("truncated output must not prove absence: %+v", o)
	}
}

func TestObserveCHAPPredatesCurrentSession(t *testing.T) {
	cfg := observationConfig(t, syntheticS1APPCAP([]byte{1, 2, 3}))
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

func TestObserveCHAPUsesPrivateCompletePrefixAndCleansIt(t *testing.T) {
	complete := syntheticS1APPCAP([]byte{1, 2, 3})
	cfg := observationConfig(t, append(complete, []byte{9, 8, 7}...))
	source := cfg.LogPath(cfg.PcapS1AP)
	var snapshotPath string
	run := func(_ context.Context, _ string, args ...string) observationCommandResult {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "chap.name") || strings.Contains(joined, "chap.value") ||
			strings.Contains(joined, "pap.password") || strings.Contains(joined, "pap.peer_id") {
			t.Fatalf("credential-bearing tshark field requested: %s", joined)
		}
		for i := range args {
			if args[i] == "-r" && i+1 < len(args) {
				snapshotPath = args[i+1]
			}
		}
		if snapshotPath == "" || snapshotPath == source {
			t.Fatal("tshark must read an immutable private prefix")
		}
		info, err := os.Stat(snapshotPath)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("snapshot permissions = %o", info.Mode().Perm())
		}
		got, err := os.ReadFile(snapshotPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(complete) {
			t.Fatalf("snapshot includes incomplete tail: %d != %d", len(got), len(complete))
		}
		return observationCommandResult{Output: []byte("1\teth:s1ap\t\n")}
	}
	o := observeCHAP(context.Background(), cfg, nil, run)
	if o.State != "unknown" || o.Reason != "capture_incomplete" ||
		!o.Capture.Incomplete || o.ScanComplete {
		t.Fatalf("unexpected incomplete-prefix observation: %+v", o)
	}
	if snapshotPath == "" {
		t.Fatal("runner was not called")
	}
	if _, err := os.Stat(snapshotPath); !os.IsNotExist(err) {
		t.Fatalf("private prefix was not removed: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(source))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".lte-s1ap-prefix-") {
			t.Fatalf("temporary prefix leaked: %s", entry.Name())
		}
	}
}

func TestObserveCHAPPreservesPositiveMetadataFromIncompletePrefix(t *testing.T) {
	cfg := observationConfig(t, append(syntheticS1APPCAP([]byte{1}), 0xaa))
	run := func(context.Context, string, ...string) observationCommandResult {
		return observationCommandResult{Output: []byte("1\teth:s1ap:chap\t2\n")}
	}
	o := observeCHAP(context.Background(), cfg, nil, run)
	if o.State != "observed" || o.Reason != "chap_frames_observed" ||
		!o.CHAPObserved || o.ScanComplete {
		t.Fatalf("unexpected positive-prefix observation: %+v", o)
	}
}

func TestObserveCHAPClassifiesSanitizedTsharkFailures(t *testing.T) {
	tests := []struct {
		stderr string
		want   string
	}{
		{"appears to have been cut short in the middle of a packet", "capture_incomplete"},
		{"Permission denied", "capture_read_failed"},
		{"Some fields aren't valid: chap.code", "tshark_incompatible"},
		{"The file isn't a capture file", "capture_format_invalid"},
		{"dissector failed", "capture_decode_failed"},
	}
	for _, tc := range tests {
		if got := classifyTsharkFailure([]byte(tc.stderr)); got != tc.want {
			t.Errorf("classify(%q) = %q, want %q", tc.stderr, got, tc.want)
		}
	}
}

func TestObserveCHAPCancellationIsDistinctAndCleansPrefix(t *testing.T) {
	cfg := observationConfig(t, syntheticS1APPCAP([]byte{1, 2, 3}))
	ctx, cancel := context.WithCancel(context.Background())
	var snapshotPath string
	run := func(runCtx context.Context, _ string, args ...string) observationCommandResult {
		for i := range args {
			if args[i] == "-r" && i+1 < len(args) {
				snapshotPath = args[i+1]
			}
		}
		cancel()
		return observationCommandResult{Err: runCtx.Err()}
	}
	o := observeCHAP(ctx, cfg, nil, run)
	if o.State != "unavailable" || o.Reason != "inspection_cancelled" {
		t.Fatalf("unexpected cancellation: %+v", o)
	}
	if snapshotPath == "" {
		t.Fatal("runner was not called")
	}
	if _, err := os.Stat(snapshotPath); !os.IsNotExist(err) {
		t.Fatalf("prefix remains after cancellation: %v", err)
	}

	preCanceled, stop := context.WithCancel(context.Background())
	stop()
	o = observeCHAP(preCanceled, cfg, nil, run)
	if o.State != "unavailable" || o.Reason != "inspection_cancelled" {
		t.Fatalf("unexpected pre-cancellation: %+v", o)
	}
}

func TestObservationCommandWaitDelayHelper(t *testing.T) {
	switch os.Getenv("LTE_OBSERVATION_WAIT_HELPER") {
	case "parent":
		cmd := exec.Command(os.Args[0], "-test.run=^TestObservationCommandWaitDelayHelper$")
		cmd.Env = append(os.Environ(), "LTE_OBSERVATION_WAIT_HELPER=descendant")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			os.Exit(91)
		}
		os.Exit(0)
	case "descendant":
		for i := 0; i < 100; i++ {
			if _, err := os.Stdout.Write([]byte(".")); err != nil {
				os.Exit(0)
			}
			time.Sleep(50 * time.Millisecond)
		}
		os.Exit(0)
	}
}

func TestRunObservationCommandBoundsInheritedPipes(t *testing.T) {
	start := time.Now()
	result := runObservationCommand(context.Background(), os.Args[0],
		"-test.run=^TestObservationCommandWaitDelayHelper$")
	// The first helper exits while a synthetic descendant retains the pipes.
	// WaitDelay must close them rather than waiting for its five-second loop.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("inherited pipes kept Wait alive for %s", elapsed)
	}
	if !errors.Is(result.Err, exec.ErrWaitDelay) || result.Failure != "tshark_wait_timeout" {
		t.Fatalf("unexpected bounded wait result: err=%v failure=%q", result.Err, result.Failure)
	}
}
