package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func telemetryJSON(now time.Time, sessions string) string {
	return fmt.Sprintf(`{"schema_version":1,"run_id":"run-1","pid":123,"started_at_unix_ms":%d,"updated_at_unix_ms":%d,"sequence":7,"state":"running","sessions":%s}`,
		now.Add(-time.Second).UnixMilli(), now.Add(-time.Second).UnixMilli(), sessions)
}

func TestReadUESnapshotCurrentMultiUE(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	sessions := `[{"session_id":"run-1:1","imsi":"001010123456789","mme_ue_s1ap_id":1,"enb_ue_s1ap_id":2,"sctp_assoc_id":3,"emm_state":"registered","ecm_state":"idle","ue_ipv4":"172.16.0.2","requested_apn":"internet","apn_source":"pdn_request","selected_apn":"internet","apn_validated":true,"bearers":[{"ebi":5,"qci":7,"state":"active"}]},{"session_id":"run-1:2","mme_ue_s1ap_id":4,"enb_ue_s1ap_id":5,"sctp_assoc_id":3,"emm_state":"common_procedure_initiated","ecm_state":"connected","apn_source":"omitted","apn_validated":false,"bearers":[]}]`
	path := filepath.Join(t.TempDir(), "ues.json")
	if err := os.WriteFile(path, []byte(telemetryJSON(now, sessions)), 0o644); err != nil {
		t.Fatal(err)
	}
	result := ReadUESnapshot(path, "run-1", 123, true, now)
	if result.State != "current" || result.Snapshot == nil || len(result.Snapshot.Sessions) != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Snapshot.Sessions[0].ECMState != "idle" || result.Snapshot.Sessions[0].EMMState != "registered" {
		t.Fatal("registered+idle session was lost")
	}
}

func TestReadUESnapshotNoCurrentStates(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	dir := t.TempDir()
	missing := ReadUESnapshot(filepath.Join(dir, "missing.json"), "run-1", 123, true, now)
	if missing.State != "missing" || missing.Snapshot != nil {
		t.Fatalf("missing: %+v", missing)
	}
	stopped := ReadUESnapshot(filepath.Join(dir, "missing.json"), "", 0, false, now)
	if stopped.State != "cell_stopped" {
		t.Fatalf("stopped: %+v", stopped)
	}
	path := filepath.Join(dir, "stale.json")
	if err := os.WriteFile(path, []byte(telemetryJSON(now.Add(-10*time.Second), `[]`)), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := ReadUESnapshot(path, "run-1", 123, true, now)
	if stale.State != "stale" {
		t.Fatalf("stale: %+v", stale)
	}
}

func TestReadUESnapshotRejectsMismatchDuplicatesAndOversize(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	valid := `{"session_id":"run-1:1","imsi":"001010123456789","mme_ue_s1ap_id":1,"enb_ue_s1ap_id":2,"sctp_assoc_id":3,"emm_state":"registered","ecm_state":"connected","apn_source":"omitted","apn_validated":false,"bearers":[]}`
	for name, payload := range map[string]string{
		"wrong run":          strings.Replace(telemetryJSON(now, `[]`), `"run_id":"run-1"`, `"run_id":"old"`, 1),
		"duplicate":          telemetryJSON(now, `[`+valid+`,`+valid+`]`),
		"bad schema":         strings.Replace(telemetryJSON(now, `[]`), `"schema_version":1`, `"schema_version":2`, 1),
		"zero sequence":      strings.Replace(telemetryJSON(now, `[]`), `"sequence":7`, `"sequence":0`, 1),
		"duplicate json key": strings.Replace(telemetryJSON(now, `[]`), `"sequence":7`, `"sequence":7,"sequence":8`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ues.json")
			if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := ReadUESnapshot(path, "run-1", 123, true, now); got.State != "invalid" {
				t.Fatalf("accepted invalid snapshot: %+v", got)
			}
		})
	}
	path := filepath.Join(t.TempDir(), "ues.json")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", int(UESnapshotMaxBytes+1))), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ReadUESnapshot(path, "run-1", 123, true, now); got.State != "invalid" {
		t.Fatalf("accepted oversize snapshot: %+v", got)
	}
}

func TestReadUESnapshotBearerQCIAndIPv4Rules(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	base := `{"session_id":"run-1:1","imsi":"001010123456789","mme_ue_s1ap_id":1,"enb_ue_s1ap_id":2,"sctp_assoc_id":3,"emm_state":"registered","ecm_state":"connected","apn_source":"omitted","apn_validated":false,"bearers":[{"ebi":5,"qci":0,"state":"requested"}]}`
	path := filepath.Join(t.TempDir(), "ues.json")
	if err := os.WriteFile(path, []byte(telemetryJSON(now, `[`+base+`]`)), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ReadUESnapshot(path, "run-1", 123, true, now); got.State != "current" {
		t.Fatalf("requested bearer with unknown QCI rejected: %+v", got)
	}
	activeUnknown := strings.Replace(base, `"state":"requested"`, `"state":"active"`, 1)
	if err := os.WriteFile(path, []byte(telemetryJSON(now, `[`+activeUnknown+`]`)), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ReadUESnapshot(path, "run-1", 123, true, now); got.State != "invalid" {
		t.Fatalf("active bearer with unknown QCI accepted: %+v", got)
	}
	mapped := strings.Replace(base, `"ecm_state":"connected"`, `"ecm_state":"connected","ue_ipv4":"::ffff:172.16.0.2"`, 1)
	if err := os.WriteFile(path, []byte(telemetryJSON(now, `[`+mapped+`]`)), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ReadUESnapshot(path, "run-1", 123, true, now); got.State != "invalid" {
		t.Fatalf("IPv4-mapped IPv6 accepted: %+v", got)
	}
}
