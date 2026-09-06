package lte

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestAlive(t *testing.T) {
	if alive(nil) {
		t.Fatal("nil cmd must not be alive")
	}
	// Exited child: start + wait, then must read as dead.
	cmd := exec.Command("sleep", "0.01")
	if err := cmd.Start(); err != nil {
		t.Skip("no sleep binary")
	}
	track(cmd)
	deadline := time.Now().Add(3 * time.Second)
	for alive(cmd) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if alive(cmd) {
		t.Fatal("exited child must not read as alive")
	}
	// Running child reads alive.
	cmd2 := exec.Command("sleep", "30")
	if err := cmd2.Start(); err != nil {
		t.Skip("no sleep binary")
	}
	defer func() { _ = cmd2.Process.Kill() }()
	track(cmd2)
	time.Sleep(100 * time.Millisecond)
	if !alive(cmd2) {
		t.Fatal("running child must read as alive")
	}
}

func TestTailFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.log")
	content := "l1\n\nl2\nl3\nl4\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := tailFile(p, 2); got != "l3 | l4" {
		t.Fatalf("want last 2 lines, got %q", got)
	}
	if got := tailFile(p, 99); got != "l1 | l2 | l3 | l4" {
		t.Fatalf("want all lines, got %q", got)
	}
	if got := tailFile(filepath.Join(t.TempDir(), "missing"), 2); got == "" {
		t.Fatal("missing file should return error text, not empty")
	}
}
