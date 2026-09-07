package lte

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/addxemmm/lte-system/internal/config"
)

func installPSHelper(t *testing.T, output string) {
	t.Helper()
	ps := copyHelper(t, "ps")
	t.Setenv("PATH", filepath.Dir(ps)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LTE_TEST_PS", output)
}

func TestRunWhileStoppedRejectsOwnedAndForeignLifecycles(t *testing.T) {
	installPSHelper(t, "PID STAT COMMAND\n")
	m := New(config.Default())
	m.natOwned = true
	called := false
	err := m.RunWhileStopped(context.Background(), func() error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrCellMustBeStopped) || called {
		t.Fatalf("owned lifecycle must reject callback: called=%v err=%v", called, err)
	}

	m.natOwned = false
	t.Setenv("LTE_TEST_PS", "PID STAT COMMAND\n4242 S srsepc\n")
	err = m.RunWhileStopped(context.Background(), func() error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrCellMustBeStopped) || called {
		t.Fatalf("foreign LTE process must reject callback: called=%v err=%v", called, err)
	}
}

func TestRunWhileStoppedSerializesStartAndStop(t *testing.T) {
	installPSHelper(t, "PID STAT COMMAND\n")
	m := New(config.Default())
	entered := make(chan struct{})
	release := make(chan struct{})
	runDone := make(chan error, 1)
	go func() {
		runDone <- m.RunWhileStopped(context.Background(), func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("callback did not start")
	}

	startCtx, cancelStart := context.WithCancel(context.Background())
	cancelStart()
	startDone := make(chan error, 1)
	go func() {
		_, err := m.Start(startCtx, StartParams{Band: "7", APN: "test", MCC: "001", MNC: "01", Network: "auto", SDR: "zmq"})
		startDone <- err
	}()
	stopDone := make(chan bool, 1)
	go func() { stopDone <- m.Stop() }()

	select {
	case err := <-startDone:
		t.Fatalf("Start escaped stopped-operation lock: %v", err)
	case stopped := <-stopDone:
		t.Fatalf("Stop escaped stopped-operation lock: %v", stopped)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-runDone; err != nil {
		t.Fatal(err)
	}
	if err := <-startDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("Start should observe cancellation after serialization: %v", err)
	}
	if stopped := <-stopDone; stopped {
		t.Fatal("idle Stop should remain a no-op")
	}
}

func TestRunWhileStoppedContextAndCallbackErrors(t *testing.T) {
	installPSHelper(t, "PID STAT COMMAND\n")
	m := New(config.Default())
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	if err := m.RunWhileStopped(canceled, func() error { called = true; return nil }); !errors.Is(err, context.Canceled) || called {
		t.Fatalf("canceled context must skip callback: called=%v err=%v", called, err)
	}
	want := errors.New("callback failure")
	if err := m.RunWhileStopped(context.Background(), func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("callback error not preserved: %v", err)
	}
	if err := m.RunWhileStopped(context.Background(), nil); err == nil {
		t.Fatal("nil callback should fail")
	}
}
