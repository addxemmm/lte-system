package crack

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

type cancelAfterChecks struct {
	context.Context
	checks int
}

func (c *cancelAfterChecks) Err() error {
	c.checks++
	if c.checks >= 3 {
		return context.Canceled
	}
	return nil
}

func (c *cancelAfterChecks) Deadline() (time.Time, bool) { return time.Time{}, false }

func (c *cancelAfterChecks) Done() <-chan struct{} { return nil }

func syntheticS1APPCAP(payloads ...[]byte) []byte {
	return syntheticClassicPCAP(binary.LittleEndian, []byte{0xd4, 0xc3, 0xb2, 0xa1}, s1apUserDLT, 65535, payloads...)
}

func syntheticClassicPCAP(order binary.ByteOrder, magic []byte, linkType, snaplen uint32, payloads ...[]byte) []byte {
	out := make([]byte, classicPCAPHeaderLen)
	copy(out[:4], magic)
	order.PutUint16(out[4:6], 2)
	order.PutUint16(out[6:8], 4)
	order.PutUint32(out[16:20], snaplen)
	order.PutUint32(out[20:24], linkType)
	for _, payload := range payloads {
		header := make([]byte, classicPCAPRecordLen)
		order.PutUint32(header[8:12], uint32(len(payload)))
		order.PutUint32(header[12:16], uint32(len(payload)))
		out = append(out, header...)
		out = append(out, payload...)
	}
	return out
}

func TestCompleteClassicPCAPPrefixEndiannessAndTimestampVariants(t *testing.T) {
	tests := []struct {
		name  string
		order binary.ByteOrder
		magic []byte
	}{
		{"little-micro", binary.LittleEndian, []byte{0xd4, 0xc3, 0xb2, 0xa1}},
		{"big-micro", binary.BigEndian, []byte{0xa1, 0xb2, 0xc3, 0xd4}},
		{"little-nano", binary.LittleEndian, []byte{0x4d, 0x3c, 0xb2, 0xa1}},
		{"big-nano", binary.BigEndian, []byte{0xa1, 0xb2, 0x3c, 0x4d}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pcap := syntheticClassicPCAP(tc.order, tc.magic, s1apUserDLT, 4096, []byte{1, 2}, []byte{3})
			end, packets, trailing, err := completeClassicPCAPPrefix(pcap)
			if err != nil || end != len(pcap) || packets != 2 || trailing {
				t.Fatalf("end=%d packets=%d trailing=%v err=%v", end, packets, trailing, err)
			}
		})
	}
}

func TestCompleteClassicPCAPPrefixDropsOnlyShortFinalRecord(t *testing.T) {
	complete := syntheticS1APPCAP([]byte{1, 2, 3})
	shortPayload := make([]byte, classicPCAPRecordLen+2)
	binary.LittleEndian.PutUint32(shortPayload[8:12], 4)
	binary.LittleEndian.PutUint32(shortPayload[12:16], 4)
	shortPayload[16], shortPayload[17] = 1, 2
	for _, tail := range [][]byte{
		{1, 2, 3},
		shortPayload,
	} {
		pcap := append(append([]byte(nil), complete...), tail...)
		end, packets, trailing, err := completeClassicPCAPPrefix(pcap)
		if err != nil || end != len(complete) || packets != 1 || !trailing {
			t.Fatalf("end=%d packets=%d trailing=%v err=%v", end, packets, trailing, err)
		}
	}
}

func TestCompleteClassicPCAPPrefixRejectsInvalidFraming(t *testing.T) {
	badVersion := syntheticS1APPCAP()
	binary.LittleEndian.PutUint16(badVersion[4:6], 3)
	zeroSnaplen := syntheticS1APPCAP()
	binary.LittleEndian.PutUint32(zeroSnaplen[16:20], 0)
	badIncluded := syntheticS1APPCAP([]byte{1})
	binary.LittleEndian.PutUint32(badIncluded[32:36], 65536)
	badOriginal := syntheticS1APPCAP([]byte{1, 2})
	binary.LittleEndian.PutUint32(badOriginal[36:40], 1)
	for name, data := range map[string][]byte{
		"magic":        append([]byte("nope"), make([]byte, 20)...),
		"version":      badVersion,
		"snaplen":      zeroSnaplen,
		"included_len": badIncluded,
		"original_len": badOriginal,
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := completeClassicPCAPPrefix(data); !errors.Is(err, errCaptureFormat) {
				t.Fatalf("got %v, want invalid format", err)
			}
		})
	}
	wrongLink := syntheticClassicPCAP(binary.LittleEndian, []byte{0xd4, 0xc3, 0xb2, 0xa1}, 1, 65535)
	if _, _, _, err := completeClassicPCAPPrefix(wrongLink); !errors.Is(err, errCaptureLinkType) {
		t.Fatalf("got %v, want unsupported link type", err)
	}
}

func TestCompleteClassicPCAPPrefixChecksCancellationWhileScanning(t *testing.T) {
	pcap := syntheticS1APPCAP()
	emptyRecord := make([]byte, classicPCAPRecordLen)
	for i := 0; i < 9000; i++ {
		pcap = append(pcap, emptyRecord...)
	}
	ctx := &cancelAfterChecks{Context: context.Background()}
	if _, _, _, err := completeClassicPCAPPrefixContext(ctx, pcap); !errors.Is(err, context.Canceled) {
		t.Fatalf("scan cancellation error = %v", err)
	}
}

func TestSnapshotClassicPCAPIsPrivateBoundedAndDetectsAppend(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.pcap")
	complete := syntheticS1APPCAP([]byte{1, 2, 3})
	if err := os.WriteFile(source, complete, 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := snapshotClassicPCAPWithHook(context.Background(), source, CHAPObservationMaxCaptureBytes, func() {
		f, openErr := os.OpenFile(source, os.O_APPEND|os.O_WRONLY, 0)
		if openErr != nil {
			t.Fatal(openErr)
		}
		_, writeErr := f.Write([]byte{9})
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatalf("append: write=%v close=%v", writeErr, closeErr)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.cleanup()
	if !snapshot.incomplete || snapshot.completePackets != 1 || snapshot.sizeBytes != int64(len(complete)) {
		t.Fatalf("unexpected snapshot metadata: %+v", snapshot)
	}
	info, err := os.Stat(snapshot.path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot mode = %o", info.Mode().Perm())
	}
	if got, err := os.ReadFile(snapshot.path); err != nil || string(got) != string(complete) {
		t.Fatalf("snapshot data mismatch: len=%d err=%v", len(got), err)
	}
}

func TestSnapshotClassicPCAPRejectsNonRegularOversizedAndCancellation(t *testing.T) {
	dir := t.TempDir()
	if _, err := snapshotClassicPCAP(context.Background(), dir, CHAPObservationMaxCaptureBytes); !errors.Is(err, errCaptureNotRegular) {
		t.Fatalf("directory error = %v", err)
	}
	large := filepath.Join(dir, "large.pcap")
	if err := os.WriteFile(large, syntheticS1APPCAP(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(large, CHAPObservationMaxCaptureBytes+1); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotClassicPCAP(context.Background(), large, CHAPObservationMaxCaptureBytes); !errors.Is(err, errCaptureTooLarge) {
		t.Fatalf("oversize error = %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := snapshotClassicPCAP(canceled, large, CHAPObservationMaxCaptureBytes); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}
}

func TestSnapshotClassicPCAPDetectsTruncateAndReplacement(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.pcap")
	complete := syntheticS1APPCAP([]byte{1, 2, 3})
	if err := os.WriteFile(source, complete, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := snapshotClassicPCAPWithHook(context.Background(), source, CHAPObservationMaxCaptureBytes, func() {
		if truncateErr := os.Truncate(source, int64(len(complete)-1)); truncateErr != nil {
			t.Fatal(truncateErr)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.incomplete || !snapshot.sourceChanged(source) {
		t.Fatalf("truncate was not detected: %+v", snapshot)
	}
	snapshot.cleanup()

	if runtime.GOOS == "windows" {
		return // Windows does not permit replacing this open source fixture.
	}
	if err := os.WriteFile(source, complete, 0o600); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(dir, "replacement.pcap")
	snapshot, err = snapshotClassicPCAPWithHook(context.Background(), source, CHAPObservationMaxCaptureBytes, func() {
		if writeErr := os.WriteFile(replacement, complete, 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
		if renameErr := os.Rename(replacement, source); renameErr != nil {
			t.Fatal(renameErr)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.cleanup()
	if snapshot.incomplete {
		t.Fatal("replacement after read does not mutate the open file metadata")
	}
	if !snapshot.sourceChanged(source) {
		t.Fatal("path replacement was not detected")
	}
}

func TestSnapshotClassicPCAPRejectsReplacementBetweenLstatAndOpen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not permit replacing this open fixture deterministically")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source.pcap")
	oldCapture := syntheticS1APPCAP([]byte{1})
	newCapture := syntheticS1APPCAP([]byte{2}, []byte{3})
	if err := os.WriteFile(source, oldCapture, 0o600); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(dir, "replacement.pcap")
	var replacedInfo os.FileInfo
	snapshot, err := snapshotClassicPCAPWithHooks(context.Background(), source, CHAPObservationMaxCaptureBytes, func() {
		if writeErr := os.WriteFile(replacement, newCapture, 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
		if renameErr := os.Rename(replacement, source); renameErr != nil {
			t.Fatal(renameErr)
		}
		replacedInfo, _ = os.Stat(source)
	}, nil)
	if !errors.Is(err, errCaptureChanged) {
		t.Fatalf("replacement error = %v", err)
	}
	if snapshot == nil || snapshot.sourceInfo == nil || replacedInfo == nil ||
		!os.SameFile(snapshot.sourceInfo, replacedInfo) {
		t.Fatal("snapshot metadata did not describe the file actually opened")
	}
	if snapshot.sourceInfo.Size() != int64(len(newCapture)) {
		t.Fatalf("opened size = %d, want %d", snapshot.sourceInfo.Size(), len(newCapture))
	}
}
