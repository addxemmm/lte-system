package sim

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/addxemmm/lte-system/internal/config"
	"github.com/addxemmm/lte-system/internal/subscriber"
)

func testResolved(t *testing.T, req WriteRequest) Resolved {
	t.Helper()
	r, err := Resolve(req, config.Default())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestResolveRejectsUnsafeDatabaseFields(t *testing.T) {
	for _, name := range []string{"ue,1", "ue\n1", "ue\r1", "ue\t1", "ue\x001", `ue"1`, " #hidden"} {
		_, err := Resolve(WriteRequest{IMSI: "001010123456789", Name: name}, config.Default())
		if err == nil {
			t.Errorf("name %q should be rejected", name)
		}
	}
	for _, req := range []WriteRequest{
		{IMSI: "001010123456789", OPType: "invalid"},
		{IMSI: "001010123456789", OPType: "opc\ninjected"},
		{IMSI: "001010123456789", OPType: "op"},
		{IMSI: "001010123456789", OP: strings.Repeat("a", 32), OPType: "opc"},
	} {
		if _, err := Resolve(req, config.Default()); err == nil {
			t.Errorf("inconsistent op_type %q should be rejected", req.OPType)
		}
	}
}

func TestAddUserExactIMSINewlineAndConflict(t *testing.T) {
	cfg := config.Default()
	cfg.ConfDir = t.TempDir()
	r := testResolved(t, WriteRequest{IMSI: "001010123456789"})
	// Target IMSI appears only in a comment and another subscriber's valid hex key.
	seed := fmt.Sprintf("# reference %s\nue0,mil,001010123456780,%s,opc,%s,8001,000000001234,7,dynamic",
		r.IMSI, r.IMSI+strings.Repeat("0", 17), r.OPValue)
	if err := os.WriteFile(cfg.UserDBPath(), []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	if st, err := AddUser(cfg.UserDBPath(), r); err != nil || st != "added" {
		t.Fatalf("exact IMSI append: status=%q error=%v", st, err)
	}
	before, rows, err := readSubscribers(cfg.UserDBPath())
	if err != nil || len(rows) != 2 || rows[1][0] != "ue1" {
		t.Fatalf("missing newline or next name: rows=%v error=%v", rows, err)
	}
	r.Ki = strings.Repeat("a", 32)
	if _, err := AddUser(cfg.UserDBPath(), r); err == nil {
		t.Fatal("conflicting authentication must be rejected")
	}
	after, err := os.ReadFile(cfg.UserDBPath())
	if err != nil || string(before) != string(after) {
		t.Fatal("conflict changed the database")
	}
	r.Name = "bad\nrow"
	if _, err := AddUser(cfg.UserDBPath(), r); err == nil {
		t.Fatal("direct Resolved caller bypassed validation")
	}
}

func TestAddUserConcurrent(t *testing.T) {
	cfg := config.Default()
	cfg.ConfDir = t.TempDir()
	const count = 32
	var wg sync.WaitGroup
	var added, exists atomic.Int32
	for i := 0; i < count; i++ {
		r := testResolved(t, WriteRequest{IMSI: fmt.Sprintf("00101012345%04d", i/2)})
		wg.Add(1)
		go func() {
			defer wg.Done()
			st, err := AddUser(cfg.UserDBPath(), r)
			if err != nil {
				t.Error(err)
			} else if st == "added" {
				added.Add(1)
			} else if st == "exists" {
				exists.Add(1)
			}
		}()
	}
	wg.Wait()
	_, rows, err := readSubscribers(cfg.UserDBPath())
	if err != nil || len(rows) != count/2 || added.Load() != count/2 || exists.Load() != count/2 {
		t.Fatalf("duplicate transaction: rows=%d added=%d exists=%d error=%v", len(rows), added.Load(), exists.Load(), err)
	}
	names := make(map[string]bool)
	for _, row := range rows {
		if names[row[0]] {
			t.Fatalf("duplicate generated name %q", row[0])
		}
		names[row[0]] = true
	}
}

func TestProgramPreflightWithoutHardware(t *testing.T) {
	for _, field := range []string{"ki", "op", "opc", "auth", "amf", "malformed", "read-error", "cancelled", "name"} {
		t.Run(field, func(t *testing.T) {
			cfg := config.Default()
			cfg.ConfDir = t.TempDir()
			req := WriteRequest{IMSI: "001010123456789"}
			r := testResolved(t, req)
			if field == "read-error" {
				if err := os.Mkdir(cfg.UserDBPath(), 0o700); err != nil {
					t.Fatal(err)
				}
			} else if _, err := AddUser(cfg.UserDBPath(), r); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch field {
			case "ki":
				req.Ki = strings.Repeat("a", 32)
			case "op":
				req.OP = strings.Repeat("a", 32)
			case "opc":
				req.OPc = strings.Repeat("a", 32)
			case "auth":
				req.Auth = "xor"
			case "amf":
				req.AMF = "1234"
			case "malformed":
				if err := os.WriteFile(cfg.UserDBPath(), []byte("bad,row\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				cancel()
			case "name":
				req.Name = "bad\nrow"
			}
			called := false
			_, _, err := program(ctx, cfg, req, func(context.Context, config.Config, Resolved) (int, string, error) {
				called = true
				return 1, "Succeed.", nil
			})
			if called || err == nil {
				t.Fatalf("preflight must reject before hardware: called=%v error=%v", called, err)
			}
		})
	}
}

func TestProgramConcurrentFakeTransactions(t *testing.T) {
	cfg := config.Default()
	cfg.ConfDir = t.TempDir()
	var wg sync.WaitGroup
	var active atomic.Int32
	for i := 0; i < 8; i++ {
		req := WriteRequest{IMSI: fmt.Sprintf("00101012345%04d", i)}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := program(context.Background(), cfg, req, func(_ context.Context, cfg config.Config, r Resolved) (int, string, error) {
				if active.Add(1) != 1 {
					t.Error("fake hardware transactions overlapped")
				}
				defer active.Add(-1)
				time.Sleep(time.Millisecond)
				_, err := addUserLocked(cfg.UserDBPath(), r)
				return 1, "Succeed.", err
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	_, rows, err := readSubscribers(cfg.UserDBPath())
	if err != nil || len(rows) != 8 {
		t.Fatalf("fake transactions lost updates: rows=%d error=%v", len(rows), err)
	}
}

func TestProgramSerializesWithDatabaseReplacement(t *testing.T) {
	cfg := config.Default()
	cfg.ConfDir = t.TempDir()
	req := WriteRequest{IMSI: "001010123456789"}
	entered := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	programDone := make(chan error, 1)
	go func() {
		_, _, err := program(context.Background(), cfg, req, func(_ context.Context, cfg config.Config, r Resolved) (int, string, error) {
			close(entered)
			<-release
			_, err := addUserLocked(cfg.UserDBPath(), r)
			return 1, "Succeed.", err
		})
		programDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("fake hardware transaction did not start")
	}
	attempted := make(chan struct{})
	replaced := make(chan struct{})
	go func() {
		close(attempted)
		subscriber.Mutex.Lock() // Same lock used by the upload handler.
		subscriber.Mutex.Unlock()
		close(replaced)
	}()
	<-attempted
	select {
	case <-replaced:
		t.Fatal("database replacement overlapped the hardware transaction")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-programDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("program deadlocked while appending")
	}
	select {
	case <-replaced:
	case <-time.After(2 * time.Second):
		t.Fatal("database replacement remained blocked")
	}
	// A duplicate with a new default SQN and hex casing still has identical credentials.
	req.Ki = strings.ToUpper(cfg.Sim.Ki)
	id, _, err := program(context.Background(), cfg, req, func(_ context.Context, cfg config.Config, r Resolved) (int, string, error) {
		st, err := addUserLocked(cfg.UserDBPath(), r)
		if st != "exists" {
			return 0, "", fmt.Errorf("want existing subscriber, got %q", st)
		}
		return 4, "exists", err
	})
	if err != nil || id != 4 {
		t.Fatalf("same credentials rejected: id=%d error=%v", id, err)
	}
}
