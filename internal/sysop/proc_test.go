package sysop

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestMain lets a copy of this test binary stand in for the kill command.
func TestMain(m *testing.M) {
	if os.Getenv("SYSOP_TEST_KILL") == "1" {
		logPath := os.Getenv("SYSOP_TEST_KILL_LOG")
		f, _ := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if f != nil {
			_, _ = f.WriteString(strings.Join(os.Args[1:], " ") + "\n")
			_ = f.Close()
		}
		state := os.Getenv("SYSOP_TEST_KILL_STATE")
		if len(os.Args) > 1 && os.Args[1] == "-9" {
			_ = os.WriteFile(state, []byte("killed"), 0o644)
			os.Exit(0)
		}
		if len(os.Args) > 1 && os.Args[1] == "-0" {
			if _, err := os.Stat(state); err == nil {
				os.Exit(1)
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestKillPIDEscalationUsesFreshContext(t *testing.T) {
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	name := "kill"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	helper := filepath.Join(dir, name)
	b, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helper, b, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "kill.log")
	statePath := filepath.Join(dir, "kill.state")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SYSOP_TEST_KILL", "1")
	t.Setenv("SYSOP_TEST_KILL_LOG", logPath)
	t.Setenv("SYSOP_TEST_KILL_STATE", statePath)

	if !killPID(4242, 20*time.Millisecond) {
		t.Fatal("KILL escalation should make the fake PID stop")
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logBytes), "-9 4242") {
		t.Fatalf("KILL was not executed with a fresh deadline:\n%s", logBytes)
	}
}
