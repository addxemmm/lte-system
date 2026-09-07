package lte

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// UESource identifies the one Manager-owned EPC telemetry source that may be
// read. Running is true only while both owned EPC and eNB children are alive.
type UESource struct {
	Running bool   `json:"running"`
	RunID   string `json:"run_id,omitempty"`
	PID     int    `json:"pid,omitempty"`
	Path    string `json:"path,omitempty"`
}

var generateRunID = randomRunID

func randomRunID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func withEnv(current []string, pairs ...string) []string {
	if len(pairs)%2 != 0 {
		panic("withEnv requires key/value pairs")
	}
	replace := make(map[string]string, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		replace[pairs[i]] = pairs[i+1]
	}
	out := make([]string, 0, len(current)+len(replace))
	for _, item := range current {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if _, replaced := replace[key]; !replaced {
			out = append(out, item)
		}
	}
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, fmt.Sprintf("%s=%s", pairs[i], pairs[i+1]))
	}
	return out
}

func (m *Manager) UESource() UESource {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !alive(m.epcCmd) || !alive(m.enbCmd) || m.epcCmd.cmd.Process == nil ||
		m.runID == "" || m.ueSnapshotPath == "" {
		return UESource{}
	}
	return UESource{
		Running: true, RunID: m.runID, PID: m.epcCmd.cmd.Process.Pid, Path: m.ueSnapshotPath,
	}
}
