package crack

import "os"

// openAppend opens log file for hashcat background output. Mode 0600 matches
// capture snapshots: job logs may echo the audited hash or status lines.
func openAppend(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	// OpenFile's mode only applies when creating a file. Tighten old logs too.
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}
