package crack

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var (
	errCaptureFormat     = errors.New("invalid classic pcap")
	errCaptureIncomplete = errors.New("incomplete classic pcap header")
	errCaptureLinkType   = errors.New("unsupported pcap link type")
	errCaptureNotRegular = errors.New("capture is not a regular file")
	errCaptureChanged    = errors.New("capture changed while being read")
	errCaptureTooLarge   = errors.New("capture exceeds size limit")
)

const (
	classicPCAPHeaderLen = 24
	classicPCAPRecordLen = 16
	s1apUserDLT          = 150
)

// captureSnapshot is an immutable, private prefix ending at a complete pcap
// record boundary. It contains no decoded packet data in Go-visible fields.
type captureSnapshot struct {
	path            string
	sourceInfo      os.FileInfo
	sizeBytes       int64
	completePackets uint64
	incomplete      bool
}

func (s *captureSnapshot) cleanup() { _ = os.Remove(s.path) }

// sourceChanged reports append, replacement, removal or timestamp mutation
// after the prefix was copied. It deliberately fails closed.
func (s *captureSnapshot) sourceChanged(source string) bool {
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(s.sourceInfo, info) {
		return true
	}
	return info.Size() != s.sourceInfo.Size() || !info.ModTime().Equal(s.sourceInfo.ModTime())
}

// snapshotClassicPCAP copies at most maxBytes from a regular classic-PCAP
// source. A concurrently written final record is omitted rather than handed to
// tshark. The returned temporary file is mode 0600 and must be cleaned up.
func snapshotClassicPCAP(ctx context.Context, source string, maxBytes int64) (*captureSnapshot, error) {
	return snapshotClassicPCAPWithHooks(ctx, source, maxBytes, nil, nil)
}

// snapshotClassicPCAPWithHook exists so tests can deterministically model an
// append between the bounded read and the second metadata check.
func snapshotClassicPCAPWithHook(ctx context.Context, source string, maxBytes int64, afterRead func()) (*captureSnapshot, error) {
	return snapshotClassicPCAPWithHooks(ctx, source, maxBytes, nil, afterRead)
}

func snapshotClassicPCAPWithHooks(ctx context.Context, source string, maxBytes int64, beforeOpen, afterRead func()) (*captureSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lstat, err := os.Lstat(source)
	if err != nil {
		return nil, err
	}
	if !lstat.Mode().IsRegular() {
		return nil, errCaptureNotRegular
	}
	if beforeOpen != nil {
		beforeOpen()
	}

	f, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	snapshot := &captureSnapshot{sourceInfo: opened}
	if !opened.Mode().IsRegular() || !os.SameFile(lstat, opened) {
		return snapshot, errCaptureChanged
	}
	if opened.Size() > maxBytes {
		return snapshot, errCaptureTooLarge
	}
	if opened.Size() == 0 && lstat.Size() != 0 {
		return snapshot, errCaptureChanged
	}

	data := make([]byte, int(opened.Size()))
	// Context cancellation is cooperative for local regular-file I/O: a Read
	// or Write already inside the kernel is not interruptible, so checks occur
	// between bounded 64 KiB chunks. No detached goroutine is used, ensuring
	// descriptors cannot outlive the request after a synthetic timeout.
	for offset := 0; offset < len(data); {
		if err := ctx.Err(); err != nil {
			return snapshot, err
		}
		end := offset + 64*1024
		if end > len(data) {
			end = len(data)
		}
		n, readErr := io.ReadFull(f, data[offset:end])
		offset += n
		if readErr != nil {
			return snapshot, errCaptureChanged
		}
	}
	if afterRead != nil {
		afterRead()
	}
	after, err := f.Stat()
	if err != nil {
		return snapshot, err
	}
	changed := after.Size() != opened.Size() || !after.ModTime().Equal(opened.ModTime())
	if len(data) == 0 {
		snapshot.incomplete = changed
		return snapshot, nil
	}

	prefixLen, packets, trailing, err := completeClassicPCAPPrefixContext(ctx, data)
	if err != nil {
		return snapshot, err
	}
	if err := ctx.Err(); err != nil {
		return snapshot, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(source), ".lte-s1ap-prefix-*.pcap")
	if err != nil {
		return snapshot, err
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return snapshot, err
	}
	for offset := 0; offset < prefixLen; {
		if err := ctx.Err(); err != nil {
			return snapshot, err
		}
		end := offset + 64*1024
		if end > prefixLen {
			end = prefixLen
		}
		n, writeErr := tmp.Write(data[offset:end])
		offset += n
		if writeErr != nil {
			return snapshot, writeErr
		}
		if n == 0 {
			return snapshot, io.ErrShortWrite
		}
	}
	if err := tmp.Close(); err != nil {
		return snapshot, err
	}
	ok = true
	snapshot.path = tmpPath
	snapshot.sizeBytes = int64(prefixLen)
	snapshot.completePackets = packets
	snapshot.incomplete = trailing || changed
	return snapshot, nil
}

// completeClassicPCAPPrefix validates framing and returns the last complete
// record boundary. A short final record is incomplete, while impossible
// framing (bad magic/version/snaplen/lengths) is invalid.
func completeClassicPCAPPrefix(data []byte) (prefixLen int, packets uint64, trailing bool, err error) {
	return completeClassicPCAPPrefixContext(context.Background(), data)
}

func completeClassicPCAPPrefixContext(ctx context.Context, data []byte) (prefixLen int, packets uint64, trailing bool, err error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, false, err
	}
	if len(data) < 4 {
		return 0, 0, true, errCaptureIncomplete
	}
	var order binary.ByteOrder
	switch string(data[:4]) {
	case "\xd4\xc3\xb2\xa1", "\x4d\x3c\xb2\xa1":
		order = binary.LittleEndian
	case "\xa1\xb2\xc3\xd4", "\xa1\xb2\x3c\x4d":
		order = binary.BigEndian
	default:
		return 0, 0, false, fmt.Errorf("%w: bad magic", errCaptureFormat)
	}
	if len(data) < classicPCAPHeaderLen {
		return 0, 0, true, errCaptureIncomplete
	}
	if order.Uint16(data[4:6]) != 2 || order.Uint16(data[6:8]) != 4 {
		return 0, 0, false, fmt.Errorf("%w: unsupported version", errCaptureFormat)
	}
	snaplen := order.Uint32(data[16:20])
	if snaplen == 0 {
		return 0, 0, false, fmt.Errorf("%w: zero snaplen", errCaptureFormat)
	}
	if order.Uint32(data[20:24])&0xffff != s1apUserDLT {
		return 0, 0, false, errCaptureLinkType
	}

	offset := classicPCAPHeaderLen
	for offset < len(data) {
		if packets%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, 0, false, err
			}
		}
		if len(data)-offset < classicPCAPRecordLen {
			return offset, packets, true, nil
		}
		included := order.Uint32(data[offset+8 : offset+12])
		original := order.Uint32(data[offset+12 : offset+16])
		if included > snaplen || included > original {
			return 0, 0, false, fmt.Errorf("%w: invalid record length", errCaptureFormat)
		}
		recordEnd64 := int64(offset) + classicPCAPRecordLen + int64(included)
		if recordEnd64 > int64(len(data)) {
			return offset, packets, true, nil
		}
		offset = int(recordEnd64)
		packets++
	}
	return offset, packets, false, nil
}
