package parser

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strings"
	"time"
)

const (
	UESnapshotMaxBytes = int64(1 << 20)
	UESnapshotMaxAge   = 5 * time.Second
	UESnapshotMaxItems = 256
	UEBearerMaxItems   = 16
)

var errDuplicateJSONField = errors.New("duplicate JSON field")

type UEBearer struct {
	EBI   uint8  `json:"ebi"`
	QCI   uint8  `json:"qci"`
	State string `json:"state"`
}

type UESession struct {
	SessionID    string     `json:"session_id"`
	IMSI         string     `json:"imsi,omitempty"`
	MMEUES1APID  uint32     `json:"mme_ue_s1ap_id"`
	ENBUES1APID  uint32     `json:"enb_ue_s1ap_id"`
	SCTPAssocID  int32      `json:"sctp_assoc_id"`
	EMMState     string     `json:"emm_state"`
	ECMState     string     `json:"ecm_state"`
	UEIPv4       string     `json:"ue_ipv4,omitempty"`
	RequestedAPN string     `json:"requested_apn,omitempty"`
	APNSource    string     `json:"apn_source"`
	SelectedAPN  string     `json:"selected_apn,omitempty"`
	APNValidated bool       `json:"apn_validated"`
	AccessPolicy string     `json:"access_policy,omitempty"`
	AccessReason string     `json:"access_reason,omitempty"`
	Bearers      []UEBearer `json:"bearers"`
}

type UESnapshot struct {
	SchemaVersion   int         `json:"schema_version"`
	RunID           string      `json:"run_id"`
	PID             int         `json:"pid"`
	StartedAtUnixMS int64       `json:"started_at_unix_ms"`
	UpdatedAtUnixMS int64       `json:"updated_at_unix_ms"`
	Sequence        uint64      `json:"sequence"`
	State           string      `json:"state"`
	Sessions        []UESession `json:"sessions"`
}

type UESnapshotResult struct {
	State    string      `json:"state"`
	Snapshot *UESnapshot `json:"snapshot,omitempty"`
	Reason   string      `json:"reason,omitempty"`
}

// ReadUESnapshot accepts only a fresh snapshot from the currently owned EPC
// process. It deliberately has no log-based fallback because independent log
// fields cannot be safely joined across concurrent UEs.
func ReadUESnapshot(path, runID string, pid int, running bool, now time.Time) UESnapshotResult {
	empty := func(state, reason string) UESnapshotResult {
		return UESnapshotResult{State: state, Reason: reason}
	}
	if !running {
		return empty("cell_stopped", "cell is not running")
	}
	if strings.TrimSpace(path) == "" || strings.TrimSpace(runID) == "" || pid <= 0 {
		return empty("missing", "current telemetry source is not available")
	}
	entry, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return empty("missing", "telemetry snapshot has not been published")
	}
	if err != nil || !entry.Mode().IsRegular() {
		return empty("invalid", "telemetry snapshot is not a regular file")
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return empty("missing", "telemetry snapshot has not been published")
	}
	if err != nil {
		return empty("invalid", "telemetry snapshot cannot be opened")
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() > UESnapshotMaxBytes {
		return empty("invalid", "telemetry snapshot metadata is invalid")
	}
	payload, err := io.ReadAll(io.LimitReader(f, UESnapshotMaxBytes+1))
	if err != nil || int64(len(payload)) > UESnapshotMaxBytes {
		return empty("invalid", "telemetry snapshot exceeds the readable limit")
	}
	afterHandle, handleErr := f.Stat()
	afterPath, pathErr := os.Lstat(path)
	if handleErr != nil || pathErr != nil || !afterPath.Mode().IsRegular() || !os.SameFile(before, afterPath) ||
		before.Size() != afterHandle.Size() || before.ModTime() != afterHandle.ModTime() ||
		afterHandle.Size() != afterPath.Size() || afterHandle.ModTime() != afterPath.ModTime() {
		return empty("invalid", "telemetry snapshot changed while being read")
	}
	if err := rejectDuplicateJSONKeys(payload); errors.Is(err, errDuplicateJSONField) {
		return empty("invalid", "telemetry snapshot contains duplicate JSON fields")
	}
	snapshot, err := decodeUESnapshot(payload)
	if err != nil {
		return empty("invalid", "telemetry snapshot is not valid schema JSON")
	}
	if reason := validateUESnapshot(snapshot, runID, pid, now); reason != "" {
		state := "invalid"
		if reason == "telemetry snapshot is stale" || snapshot.State == "stopped" {
			state = "stale"
		}
		return empty(state, reason)
	}
	if snapshot.Sessions == nil {
		snapshot.Sessions = []UESession{}
	}
	return UESnapshotResult{State: "current", Snapshot: &snapshot}
}

func decodeUESnapshot(payload []byte) (UESnapshot, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil || fields == nil {
		return UESnapshot{}, errors.New("telemetry root must be an object")
	}

	var snapshot UESnapshot
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return UESnapshot{}, err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return UESnapshot{}, errors.New("telemetry snapshot contains trailing data")
	}

	var rawSessions []json.RawMessage
	if raw, exists := fields["sessions"]; exists && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if err := json.Unmarshal(raw, &rawSessions); err != nil {
			return UESnapshot{}, err
		}
	}
	if len(rawSessions) != len(snapshot.Sessions) {
		return UESnapshot{}, errors.New("telemetry sessions could not be decoded")
	}
	for _, raw := range rawSessions {
		var sessionFields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &sessionFields); err != nil || sessionFields == nil {
			return UESnapshot{}, errors.New("telemetry session must be an object")
		}
		switch snapshot.SchemaVersion {
		case 1:
			for name := range sessionFields {
				if strings.EqualFold(name, "access_policy") || strings.EqualFold(name, "access_reason") {
					return UESnapshot{}, errors.New("schema v1 session contains a schema v2 access field")
				}
			}
		case 2:
			for _, name := range []string{"access_policy", "access_reason", "apn_validated"} {
				rawValue, exists := sessionFields[name]
				if !exists || jsonValueIsNull(rawValue) {
					return UESnapshot{}, fmt.Errorf("schema v2 session requires non-null %s", name)
				}
			}
			for name, rawValue := range sessionFields {
				if !jsonValueIsNull(rawValue) {
					continue
				}
				// Match encoding/json's case-insensitive field lookup.
				for _, stringField := range []string{
					"session_id", "imsi", "emm_state", "ecm_state", "ue_ipv4",
					"requested_apn", "apn_source", "selected_apn", "access_policy", "access_reason",
				} {
					if strings.EqualFold(name, stringField) {
						return UESnapshot{}, fmt.Errorf("schema v2 session field %s must not be null", name)
					}
				}
			}
		}
	}
	return snapshot, nil
}
func jsonValueIsNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
func validateUESnapshot(snapshot UESnapshot, runID string, pid int, now time.Time) string {
	switch {
	case snapshot.SchemaVersion != 1 && snapshot.SchemaVersion != 2:
		return "unsupported telemetry schema version"
	case snapshot.RunID != runID || snapshot.PID != pid:
		return "telemetry snapshot does not belong to the current cell run"
	case snapshot.Sequence == 0:
		return "telemetry sequence must be positive"
	case snapshot.State == "stopped":
		return "telemetry snapshot is stale"
	case snapshot.State != "running":
		return "unknown telemetry state"
	case snapshot.StartedAtUnixMS <= 0 || snapshot.UpdatedAtUnixMS < snapshot.StartedAtUnixMS:
		return "telemetry timestamps are invalid"
	case snapshot.Sessions == nil:
		return "sessions array is required"
	case len(snapshot.Sessions) > UESnapshotMaxItems:
		return "telemetry session limit exceeded"
	}
	updated := time.UnixMilli(snapshot.UpdatedAtUnixMS)
	if updated.After(now) {
		return "telemetry timestamp is in the future"
	}
	if now.Sub(updated) > UESnapshotMaxAge {
		return "telemetry snapshot is stale"
	}
	seenSession, seenIMSI, seenIPv4 := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	for i, session := range snapshot.Sessions {
		if reason := validateUESession(session, snapshot.SchemaVersion); reason != "" {
			return fmt.Sprintf("session %d is invalid: %s", i, reason)
		}
		if _, exists := seenSession[session.SessionID]; exists {
			return "telemetry contains duplicate session_id"
		}
		if !strings.HasPrefix(session.SessionID, snapshot.RunID+":") {
			return "telemetry session_id does not belong to the current run"
		}
		seenSession[session.SessionID] = struct{}{}
		if session.IMSI != "" {
			if _, exists := seenIMSI[session.IMSI]; exists {
				return "telemetry contains duplicate imsi"
			}
			seenIMSI[session.IMSI] = struct{}{}
		}
		if session.UEIPv4 != "" {
			if _, exists := seenIPv4[session.UEIPv4]; exists {
				return "telemetry contains duplicate ue_ipv4"
			}
			seenIPv4[session.UEIPv4] = struct{}{}
		}
	}
	return ""
}

func validateUESession(session UESession, schemaVersion int) string {
	if len(session.SessionID) == 0 || len(session.SessionID) > 128 || strings.ContainsAny(session.SessionID, "\r\n") {
		return "session_id must be 1-128 characters"
	}
	if session.IMSI != "" && !strictIMSI(session.IMSI) {
		return "imsi must be empty or exactly 15 digits"
	}
	if !oneOf(session.EMMState, "deregistered", "common_procedure_initiated", "registered", "deregistered_initiated") {
		return "unknown emm_state"
	}
	if !oneOf(session.ECMState, "idle", "connected") {
		return "unknown ecm_state"
	}
	if session.UEIPv4 != "" {
		ip, err := netip.ParseAddr(session.UEIPv4)
		if err != nil || !ip.Is4() {
			return "ue_ipv4 is not IPv4"
		}
	}
	if !oneOf(session.APNSource, "omitted", "pdn_request", "esm_information_response") {
		return "unknown apn_source"
	}
	if session.APNSource != "omitted" && session.RequestedAPN == "" {
		return "requested_apn is required for its apn_source"
	}
	if session.APNSource == "omitted" && session.RequestedAPN != "" {
		return "requested_apn conflicts with omitted apn_source"
	}
	if len(session.RequestedAPN) > 99 || len(session.SelectedAPN) > 99 {
		return "apn exceeds length limit"
	}
	if strings.ContainsAny(session.RequestedAPN+session.SelectedAPN, "\x00\r\n") {
		return "apn contains control characters"
	}
	if schemaVersion == 1 {
		if session.SelectedAPN != "" && !session.APNValidated {
			return "selected_apn requires apn_validated"
		}
		if session.APNValidated && session.SelectedAPN == "" {
			return "apn_validated requires selected_apn"
		}
	} else if reason := validateUESessionAccess(session); reason != "" {
		return reason
	}
	if len(session.Bearers) > UEBearerMaxItems {
		return "bearer limit exceeded"
	}
	if session.Bearers == nil {
		return "bearers array is required"
	}
	seenEBI := map[uint8]struct{}{}
	for _, bearer := range session.Bearers {
		qciUnknownAllowed := bearer.QCI == 0 && oneOf(bearer.State, "deactivated", "requested")
		if bearer.EBI < 5 || bearer.EBI > 15 || (!qciUnknownAllowed && (bearer.QCI < 1 || bearer.QCI > 9)) ||
			!oneOf(bearer.State, "deactivated", "requested", "setup", "active") {
			return "invalid bearer"
		}
		if _, exists := seenEBI[bearer.EBI]; exists {
			return "duplicate bearer ebi"
		}
		seenEBI[bearer.EBI] = struct{}{}
	}
	return ""
}

func validateUESessionAccess(session UESession) string {
	if session.RequestedAPN != "" {
		if err := validateTelemetryAPN(session.RequestedAPN); err != nil {
			return "requested_apn is invalid"
		}
	}
	if session.SelectedAPN != "" {
		if err := validateTelemetryAPN(session.SelectedAPN); err != nil {
			return "selected_apn is invalid"
		}
	}

	switch session.AccessPolicy {
	case "normal":
		if !session.APNValidated || session.SelectedAPN == "" {
			return "normal access requires a validated selected_apn"
		}
		if session.APNSource == "omitted" {
			if session.AccessReason != "apn_omitted" {
				return "normal omitted APN requires apn_omitted reason"
			}
		} else if !strings.EqualFold(session.RequestedAPN, session.SelectedAPN) || session.AccessReason != "apn_match" {
			return "normal requested APN requires matching selection and apn_match reason"
		}
	case "restricted":
		if session.APNValidated || session.APNSource == "omitted" || session.RequestedAPN == "" || session.SelectedAPN == "" {
			return "restricted access requires explicit unvalidated requested and selected APNs"
		}
		if strings.EqualFold(session.RequestedAPN, session.SelectedAPN) || session.AccessReason != "apn_mismatch" {
			return "restricted access requires an APN mismatch and apn_mismatch reason"
		}
	case "deny":
		if session.APNValidated || session.SelectedAPN != "" || session.AccessReason != "session_unavailable" {
			return "deny access requires empty selection and session_unavailable reason"
		}
	default:
		return "unknown access_policy"
	}
	return ""
}

func validateTelemetryAPN(apn string) error {
	if len(apn) == 0 || len(apn) > 99 {
		return errors.New("APN length is invalid")
	}
	for _, label := range strings.Split(apn, ".") {
		if len(label) == 0 || len(label) > 63 {
			return errors.New("APN label length is invalid")
		}
		for i, r := range label {
			alphaNum := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
			if !alphaNum && !(r == '-' && i > 0 && i < len(label)-1) {
				return errors.New("APN label contains an invalid character")
			}
		}
	}
	return nil
}
func strictIMSI(value string) bool {
	if len(value) != 15 {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func rejectDuplicateJSONKeys(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := walkJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

func walkJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return errDuplicateJSONField
			}
			seen[key] = struct{}{}
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return errors.New("unexpected JSON delimiter")
	}
}
