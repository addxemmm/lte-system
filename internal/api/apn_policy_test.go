package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/addxemmm/lte-system/internal/lte"
)

func TestV1APNMismatchPolicyValidationDoesNotStartCell(t *testing.T) {
	s, _ := testServer(t)
	if err := s.mgr.SaveProfile(lte.StartParams{
		Band: "7", APN: "internet", APNMismatchPolicy: lte.APNMismatchRestricted,
		MCC: "001", MNC: "01", Network: "auto",
	}); err != nil {
		t.Fatal(err)
	}
	for _, policy := range []string{"fallback", " \t"} {
		t.Run(policy, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{
				"band": "7", "apn": "internet", "apn_mismatch_policy": policy,
				"mcc": "001", "mnc": "01", "network": "auto", "sdr": "zmq",
			})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/cell", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("invalid policy status = %d: %s", rec.Code, rec.Body.String())
			}
			var envelope map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			data, _ := envelope["data"].(map[string]any)
			errorsList, _ := data["errors"].([]any)
			found := false
			for _, raw := range errorsList {
				issue, _ := raw.(map[string]any)
				if issue["field"] == "apn_mismatch_policy" {
					found = true
				}
			}
			if !found {
				t.Fatalf("policy validation issue missing: %s", rec.Body.String())
			}
			if s.mgr.IsRunning().Running {
				t.Fatal("validation request started a cell")
			}
		})
	}
}
func TestV1ProfileShowsEffectiveAPNMismatchPolicy(t *testing.T) {
	s, _ := testServer(t)
	if err := s.mgr.SaveProfile(lte.StartParams{
		Band: "7", APN: "internet", MCC: "001", MNC: "01", Network: "auto",
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/profile", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("profile status = %d: %s", rec.Code, rec.Body.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data, _ := envelope["data"].(map[string]any)
	profile, _ := data["profile"].(map[string]any)
	if profile["apn_mismatch_policy"] != lte.APNMismatchStrict {
		t.Fatalf("profile policy not effective strict: %v", profile)
	}
}
