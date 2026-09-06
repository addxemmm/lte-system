//go:build linux

package crack

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/addxemmm/lte-system/internal/config"
)

// A short-lived shell fixture substitutes the configured binary; hashcat is never run.
func TestStartAsyncClosesLogsAndReaps(t *testing.T) {
	dir := t.TempDir()
	pidfile := filepath.Join(dir, "children")
	t.Setenv("LTE_TEST_PIDFILE", pidfile)
	helper := filepath.Join(dir, "helper")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\necho $$ >> \"$LTE_TEST_PIDFILE\"\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.LogDir, cfg.DataDir, cfg.HashcatBin = dir, dir, helper
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 24; i++ {
		if err := StartAsync(cfg, "aabbcc:112233:01"); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		content, _ := os.ReadFile(pidfile)
		pids := strings.Fields(string(content))
		reaped := len(pids) == 24
		for _, pid := range pids {
			if _, err := os.Stat("/proc/" + pid); !os.IsNotExist(err) {
				reaped = false
			}
		}
		if reaped {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture children were not reaped")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cfg.HashcatBin = filepath.Join(dir, "missing")
	for i := 0; i < 24; i++ {
		if err := StartAsync(cfg, "aabbcc:112233:01"); err == nil {
			t.Fatal("missing helper started")
		}
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) > len(before)+2 {
		t.Fatal(fmt.Sprintf("log descriptors leaked: before=%d after=%d", len(before), len(after)))
	}
}
