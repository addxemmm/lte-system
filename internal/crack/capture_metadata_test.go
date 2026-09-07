package crack

import (
	"context"
	"os"
	"testing"
)

func TestCaptureIntegrityMetadataDistinguishesTailAndMutation(t *testing.T) {
	for _, tc := range []struct {
		name         string
		tail, mutate bool
	}{
		{"complete", false, false},
		{"stable-tail", true, false},
		{"changed-source", false, true},
		{"tail-and-change", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := syntheticS1APPCAP([]byte{1, 2, 3})
			prefixSize := len(data)
			if tc.tail {
				data = append(data, 1, 2, 3)
			}
			cfg := observationConfig(t, data)
			run := func(context.Context, string, ...string) observationCommandResult {
				if tc.mutate {
					if err := os.WriteFile(cfg.LogPath(cfg.PcapS1AP), append(data, 4), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				return observationCommandResult{Output: []byte("1\ts1ap\t\n")}
			}
			o := observeCHAP(context.Background(), cfg, nil, run)
			wantTrailing := int64(0)
			if tc.tail {
				wantTrailing = 3
			}
			if o.Capture.TailIncomplete != tc.tail || o.Capture.SourceChanged != tc.mutate || o.Capture.TrailingBytes != wantTrailing {
				t.Fatalf("wrong integrity metadata: %+v", o.Capture)
			}
			if o.Capture.SnapshotSize != int64(prefixSize) || o.Capture.CompletePackets != 1 {
				t.Fatalf("wrong framing counts: %+v", o.Capture)
			}
			if tc.tail || tc.mutate {
				if o.ScanComplete || o.State != "unknown" || o.Reason != "capture_incomplete" {
					t.Fatalf("partial scan became conclusive: %+v", o)
				}
			} else if !o.ScanComplete {
				t.Fatalf("complete scan became incomplete: %+v", o)
			}
		})
	}
}

func TestCaptureIntegrityMetadataShortHeader(t *testing.T) {
	cfg := observationConfig(t, []byte{0xd4, 0xc3})
	o := observeCHAP(context.Background(), cfg, nil, nil)
	if !o.Capture.TailIncomplete || o.Capture.SourceChanged || o.ScanComplete || o.Capture.TrailingBytes != 0 {
		t.Fatalf("unexpected short-header metadata: %+v", o)
	}
}

func TestSnapshotMetadataMutationDuringRead(t *testing.T) {
	data := syntheticS1APPCAP([]byte{1})
	cfg := observationConfig(t, data)
	source := cfg.LogPath(cfg.PcapS1AP)
	s, err := snapshotClassicPCAPWithHook(context.Background(), source, CHAPObservationMaxCaptureBytes, func() {
		if err := os.WriteFile(source, append(data, 1), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.cleanup()
	if !s.sourceMutated || s.tailIncomplete || s.trailingBytes != 0 || !s.incomplete {
		t.Fatalf("unexpected metadata: %+v", s)
	}
}
