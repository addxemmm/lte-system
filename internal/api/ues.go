package api

import (
	"net/http"
	"time"

	"github.com/addxemmm/lte-system/internal/parser"
)

func (s *Server) handleV1UEs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeV1(w, r, CodeMethod, "method not allowed, want GET", nil)
		return
	}
	result := s.currentUESnapshot()
	writeV1(w, r, CodeOK, "ok", ueSnapshotData(result))
}

func (s *Server) handleV1UE(w http.ResponseWriter, r *http.Request, wanted string) {
	if r.Method != http.MethodGet {
		writeV1(w, r, CodeMethod, "method not allowed, want GET", nil)
		return
	}
	if !validIMSI(wanted) {
		writeValidation(w, r, "imsi", "must contain exactly 15 digits")
		return
	}
	result := s.currentUESnapshot()
	if result.State != "current" || result.Snapshot == nil {
		writeV1(w, r, CodeOK, "current UE telemetry is not available", ueSnapshotData(result))
		return
	}
	for _, session := range result.Snapshot.Sessions {
		if session.IMSI == wanted {
			writeV1(w, r, CodeOK, "ok", map[string]any{
				"state": "current", "sequence": result.Snapshot.Sequence,
				"updated_at_unix_ms": result.Snapshot.UpdatedAtUnixMS, "session": session,
			})
			return
		}
	}
	writeV1(w, r, CodeNotFound, "UE is not present in the complete current snapshot", nil)
}

func (s *Server) currentUESnapshot() parser.UESnapshotResult {
	before := s.mgr.UESource()
	result := parser.ReadUESnapshot(before.Path, before.RunID, before.PID, before.Running, time.Now())
	after := s.mgr.UESource()
	if before != after {
		state, reason := "invalid", "cell run changed while telemetry was being read"
		if !after.Running {
			state, reason = "cell_stopped", "cell stopped while telemetry was being read"
		}
		return parser.UESnapshotResult{State: state, Reason: reason}
	}
	return result
}

func ueSnapshotData(result parser.UESnapshotResult) map[string]any {
	if result.State != "current" || result.Snapshot == nil {
		return map[string]any{
			"state": result.State, "reason": result.Reason,
			"sessions": []parser.UESession{}, "count": 0,
		}
	}
	snapshot := result.Snapshot
	return map[string]any{
		"state": "current", "cell_state": snapshot.State, "schema_version": snapshot.SchemaVersion,
		"run_id": snapshot.RunID, "sequence": snapshot.Sequence,
		"started_at_unix_ms": snapshot.StartedAtUnixMS, "updated_at_unix_ms": snapshot.UpdatedAtUnixMS,
		"sessions": snapshot.Sessions, "count": len(snapshot.Sessions),
	}
}
