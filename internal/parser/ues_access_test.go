package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func accessTelemetryJSON(now time.Time, schema int, sessions string) string {
	return fmt.Sprintf(`{"schema_version":%d,"run_id":"run-1","pid":123,"started_at_unix_ms":%d,"updated_at_unix_ms":%d,"sequence":9,"state":"running","sessions":%s}`,
		schema, now.Add(-time.Second).UnixMilli(), now.Add(-time.Second).UnixMilli(), sessions)
}

func readAccessFixture(t *testing.T, now time.Time, payload string) UESnapshotResult {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ues.json")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	return ReadUESnapshot(path, "run-1", 123, true, now)
}

func TestReadUESnapshotSchemaV1StrictCompatibility(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	strictMismatch := `{"session_id":"run-1:1","imsi":"001010123456789","mme_ue_s1ap_id":1,"enb_ue_s1ap_id":2,"sctp_assoc_id":3,"emm_state":"common_procedure_initiated","ecm_state":"connected","requested_apn":"other","apn_source":"pdn_request","apn_validated":false,"bearers":[]}`
	if got := readAccessFixture(t, now, accessTelemetryJSON(now, 1, `[`+strictMismatch+`]`)); got.State != "current" {
		t.Fatalf("schema v1 strict mismatch compatibility was lost: %+v", got)
	}
	withV2Field := strings.Replace(strictMismatch, `"apn_validated":false`, `"apn_validated":false,"access_policy":"deny"`, 1)
	if got := readAccessFixture(t, now, accessTelemetryJSON(now, 1, `[`+withV2Field+`]`)); got.State != "invalid" {
		t.Fatalf("schema v1 accepted a schema v2 field: %+v", got)
	}
}

func TestReadUESnapshotSchemaV2NormalRestrictedAndDeny(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	sessions := `[` +
		`{"session_id":"run-1:1","imsi":"001010123456781","mme_ue_s1ap_id":1,"enb_ue_s1ap_id":11,"sctp_assoc_id":3,"emm_state":"registered","ecm_state":"idle","requested_apn":"Internet","apn_source":"pdn_request","selected_apn":"internet","apn_validated":true,"access_policy":"normal","access_reason":"apn_match","bearers":[]},` +
		`{"session_id":"run-1:2","imsi":"001010123456782","mme_ue_s1ap_id":2,"enb_ue_s1ap_id":12,"sctp_assoc_id":3,"emm_state":"registered","ecm_state":"connected","apn_source":"omitted","selected_apn":"internet","apn_validated":true,"access_policy":"normal","access_reason":"apn_omitted","bearers":[]},` +
		`{"session_id":"run-1:3","imsi":"001010123456783","mme_ue_s1ap_id":3,"enb_ue_s1ap_id":13,"sctp_assoc_id":3,"emm_state":"common_procedure_initiated","ecm_state":"connected","requested_apn":"other","apn_source":"esm_information_response","selected_apn":"internet","apn_validated":false,"access_policy":"restricted","access_reason":"apn_mismatch","bearers":[]},` +
		`{"session_id":"run-1:4","imsi":"001010123456784","mme_ue_s1ap_id":4,"enb_ue_s1ap_id":14,"sctp_assoc_id":3,"emm_state":"deregistered","ecm_state":"idle","requested_apn":"other","apn_source":"pdn_request","apn_validated":false,"access_policy":"deny","access_reason":"session_unavailable","bearers":[]}` +
		`]`
	got := readAccessFixture(t, now, accessTelemetryJSON(now, 2, sessions))
	if got.State != "current" || got.Snapshot == nil || len(got.Snapshot.Sessions) != 4 {
		t.Fatalf("valid schema v2 policies rejected: %+v", got)
	}
	if got.Snapshot.Sessions[2].AccessPolicy != "restricted" || got.Snapshot.Sessions[2].APNValidated {
		t.Fatalf("restricted decision was not preserved: %+v", got.Snapshot.Sessions[2])
	}
	if got.Snapshot.Sessions[3].SelectedAPN != "" || got.Snapshot.Sessions[3].AccessReason != "session_unavailable" {
		t.Fatalf("deny decision was not preserved: %+v", got.Snapshot.Sessions[3])
	}
}

func TestReadUESnapshotSchemaV2DualUEPoliciesAreIndependent(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	normal := `{"session_id":"run-1:1","imsi":"001010123456781","mme_ue_s1ap_id":1,"enb_ue_s1ap_id":11,"sctp_assoc_id":3,"emm_state":"registered","ecm_state":"connected","requested_apn":"internet","apn_source":"pdn_request","selected_apn":"INTERNET","apn_validated":true,"access_policy":"normal","access_reason":"apn_match","bearers":[]}`
	restricted := `{"session_id":"run-1:2","imsi":"001010123456782","mme_ue_s1ap_id":2,"enb_ue_s1ap_id":12,"sctp_assoc_id":4,"emm_state":"common_procedure_initiated","ecm_state":"connected","requested_apn":"wrong","apn_source":"pdn_request","selected_apn":"internet","apn_validated":false,"access_policy":"restricted","access_reason":"apn_mismatch","bearers":[]}`
	got := readAccessFixture(t, now, accessTelemetryJSON(now, 2, `[`+normal+`,`+restricted+`]`))
	if got.State != "current" || got.Snapshot.Sessions[0].AccessPolicy != "normal" || got.Snapshot.Sessions[1].AccessPolicy != "restricted" {
		t.Fatalf("per-UE access decisions were conflated: %+v", got)
	}
}

func TestReadUESnapshotSchemaV2RejectsMissingNullUnknownAndContradictions(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	valid := `{"session_id":"run-1:1","imsi":"001010123456789","mme_ue_s1ap_id":1,"enb_ue_s1ap_id":2,"sctp_assoc_id":3,"emm_state":"registered","ecm_state":"connected","requested_apn":"wrong","apn_source":"pdn_request","selected_apn":"internet","apn_validated":false,"access_policy":"restricted","access_reason":"apn_mismatch","bearers":[]}`
	cases := map[string]string{
		"missing policy":       strings.Replace(valid, `,"access_policy":"restricted"`, ``, 1),
		"missing reason":       strings.Replace(valid, `,"access_reason":"apn_mismatch"`, ``, 1),
		"missing validated":    strings.Replace(valid, `,"apn_validated":false`, ``, 1),
		"null policy":          strings.Replace(valid, `"access_policy":"restricted"`, `"access_policy":null`, 1),
		"null reason":          strings.Replace(valid, `"access_reason":"apn_mismatch"`, `"access_reason":null`, 1),
		"null validated":       strings.Replace(valid, `"apn_validated":false`, `"apn_validated":null`, 1),
		"null selected APN":    strings.Replace(valid, `"selected_apn":"internet"`, `"selected_apn":null`, 1),
		"null requested APN":   strings.Replace(valid, `"requested_apn":"wrong"`, `"requested_apn":null`, 1),
		"null APN source":      strings.Replace(valid, `"apn_source":"pdn_request"`, `"apn_source":null`, 1),
		"null other string":    strings.Replace(valid, `"imsi":"001010123456789"`, `"imsi":null`, 1),
		"unknown field":        strings.Replace(valid, `"bearers":[]`, `"unexpected":true,"bearers":[]`, 1),
		"unknown policy":       strings.Replace(valid, `"access_policy":"restricted"`, `"access_policy":"allow"`, 1),
		"restricted validated": strings.Replace(valid, `"apn_validated":false`, `"apn_validated":true`, 1),
		"restricted match":     strings.Replace(valid, `"requested_apn":"wrong"`, `"requested_apn":"INTERNET"`, 1),
		"restricted reason":    strings.Replace(valid, `"access_reason":"apn_mismatch"`, `"access_reason":"apn_match"`, 1),
		"malformed APN":        strings.Replace(valid, `"requested_apn":"wrong"`, `"requested_apn":"bad..apn"`, 1),
		"normal mismatch": strings.NewReplacer(
			`"access_policy":"restricted"`, `"access_policy":"normal"`,
			`"access_reason":"apn_mismatch"`, `"access_reason":"apn_match"`,
			`"apn_validated":false`, `"apn_validated":true`,
		).Replace(valid),
		"deny selected": strings.NewReplacer(
			`"access_policy":"restricted"`, `"access_policy":"deny"`,
			`"access_reason":"apn_mismatch"`, `"access_reason":"session_unavailable"`,
		).Replace(valid),
	}
	for name, session := range cases {
		t.Run(name, func(t *testing.T) {
			got := readAccessFixture(t, now, accessTelemetryJSON(now, 2, `[`+session+`]`))
			if got.State != "invalid" {
				t.Fatalf("accepted contradictory schema v2 session: %+v", got)
			}
		})
	}
}

func TestReadUESnapshotAccessStringNullCaseVariants(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	base := `{"session_id":"run-1:1","emm_state":"deregistered","ecm_state":"idle","apn_source":"omitted","apn_validated":false,"bearers":[]}`
	for _, schema := range []int{1, 2} {
		t.Run(fmt.Sprintf("schema%d", schema), func(t *testing.T) {
			session := base
			if schema == 2 {
				session = strings.TrimSuffix(session, "}") + `,"access_policy":"deny","access_reason":"session_unavailable"}`
			}
			if got := readAccessFixture(t, now, accessTelemetryJSON(now, schema, `[`+session+`]`)); got.State != "current" {
				t.Fatalf("valid empty-selection session rejected: %+v", got)
			}
			for _, key := range []string{
				"selected_apn", "Selected_APN", "SELECTED_APN",
				"requested_apn", "Requested_APN", "REQUESTED_APN",
			} {
				t.Run(key, func(t *testing.T) {
					withNull := strings.TrimSuffix(session, "}") + fmt.Sprintf(`,"%s":null}`, key)
					got := readAccessFixture(t, now, accessTelemetryJSON(now, schema, `[`+withNull+`]`))
					want := "invalid"
					if schema == 1 {
						want = "current" // Preserve legacy string-null decoding.
					}
					if got.State != want {
						t.Fatalf("schema %d %s=null: want %s, got %+v", schema, key, want, got)
					}
				})
			}
		})
	}
}
