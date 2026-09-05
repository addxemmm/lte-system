// Package sysop provides process helpers without shell injection.
// Legacy used `ps -aux | grep srs` which can match itself; we use pgrep/exact match + pidfiles.
package sysop

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Running reports whether any process with the exact name is alive.
func Running(name string) bool {
	return len(PIDs(name)) > 0
}

// PIDs returns PIDs for an exact process name via `pgrep -x`, falling back to ps scan.
func PIDs(name string) []int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "pgrep", "-x", name).Output(); err == nil {
		return parsePIDList(string(out))
	}
	// Fallback: ps -eo pid,comm and exact-match comm.
	out, err := exec.CommandContext(ctx, "ps", "-eo", "pid,comm").Output()
	if err != nil {
		return nil
	}
	var res []int
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		if f[1] == name {
			if pid, err := strconv.Atoi(f[0]); err == nil {
				res = append(res, pid)
			}
		}
	}
	return res
}

func parsePIDList(s string) []int {
	var res []int
	for _, f := range strings.Fields(s) {
		if pid, err := strconv.Atoi(strings.TrimSpace(f)); err == nil {
			res = append(res, pid)
		}
	}
	return res
}

// AnyRunning reports true if any of the names is running.
func AnyRunning(names ...string) (string, bool) {
	for _, n := range names {
		if Running(n) {
			return n, true
		}
	}
	return "", false
}

// KillAll sends TERM then KILL to all PIDs of name. Returns killed count.
func KillAll(name string, timeout time.Duration) int {
	pids := PIDs(name)
	killed := 0
	for _, pid := range pids {
		if killPID(pid, timeout) {
			killed++
		}
	}
	return killed
}

func killPID(pid int, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// TERM first
	_ = exec.CommandContext(ctx, "kill", strconv.Itoa(pid)).Run()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	_ = exec.CommandContext(ctx, "kill", "-9", strconv.Itoa(pid)).Run()
	time.Sleep(300 * time.Millisecond)
	return !pidAlive(pid)
}

func pidAlive(pid int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "kill", "-0", strconv.Itoa(pid)).Run() == nil
}
