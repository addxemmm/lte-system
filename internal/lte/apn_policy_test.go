package lte

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/addxemmm/lte-system/internal/config"
)

func validAPNPolicyParams() StartParams {
	return StartParams{Band: "7", APN: "internet", MCC: "001", MNC: "01", Network: "eth0"}
}

func TestAPNMismatchPolicyValidationAndDefaults(t *testing.T) {
	base := validAPNPolicyParams()
	if err := base.Validate(); err != nil {
		t.Fatalf("empty policy must default to strict: %v", err)
	}
	normalizeAPNMismatchPolicy(&base)
	if base.APNMismatchPolicy != APNMismatchStrict {
		t.Fatalf("default policy = %q, want strict", base.APNMismatchPolicy)
	}

	restricted := validAPNPolicyParams()
	restricted.APNMismatchPolicy = APNMismatchRestricted
	if err := restricted.Validate(); err != nil {
		t.Fatalf("restricted policy rejected: %v", err)
	}

	for _, invalid := range []string{"allow", "STRICT", " strict ", " ", "\t"} {
		p := validAPNPolicyParams()
		p.APNMismatchPolicy = invalid
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "apn_mismatch_policy") {
			t.Fatalf("invalid policy %q was not attributed: %v", invalid, err)
		}
		issues := p.ValidateDetailed()
		found := false
		for _, issue := range issues {
			if issue.Field == "apn_mismatch_policy" {
				found = true
			}
		}
		if !found {
			t.Fatalf("detailed validation omitted policy issue for %q: %+v", invalid, issues)
		}
	}
}

func TestAPNMismatchPolicyProfileRoundTripOverlayAndLegacy(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.ConfDir = filepath.Join(cfg.DataDir, "conf")
	cfg.LogDir = filepath.Join(cfg.DataDir, "log")
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)

	profile := validAPNPolicyParams()
	profile.APNMismatchPolicy = APNMismatchRestricted
	if err := m.SaveProfile(profile); err != nil {
		t.Fatal(err)
	}
	loaded, ok := m.LoadProfile()
	if !ok || loaded.APNMismatchPolicy != APNMismatchRestricted {
		t.Fatalf("restricted profile did not round-trip: %v %+v", ok, loaded)
	}
	inherited := StartParams{Band: "40"}
	if !m.OverlayProfile(&inherited) || inherited.APNMismatchPolicy != APNMismatchRestricted {
		t.Fatalf("empty request did not inherit restricted: %+v", inherited)
	}
	explicitStrict := StartParams{APNMismatchPolicy: APNMismatchStrict}
	if !m.OverlayProfile(&explicitStrict) || explicitStrict.APNMismatchPolicy != APNMismatchStrict {
		t.Fatalf("explicit strict did not override restricted: %+v", explicitStrict)
	}
	whitespace := StartParams{APNMismatchPolicy: " \t"}
	if !m.OverlayProfile(&whitespace) || whitespace.APNMismatchPolicy != " \t" {
		t.Fatalf("whitespace request was replaced by saved restricted: %+v", whitespace)
	}
	if issues := whitespace.ValidateDetailed(); !hasFieldIssue(issues, "apn_mismatch_policy") {
		t.Fatalf("whitespace request was not rejected: %+v", issues)
	}

	legacy := map[string]any{"band": "7", "apn": "internet", "mcc": "001", "mnc": "01"}
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.ProfilePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, ok = m.LoadProfile()
	if !ok || loaded.APNMismatchPolicy != APNMismatchStrict {
		t.Fatalf("legacy profile did not upgrade to strict: %v %+v", ok, loaded)
	}

	for _, invalid := range []string{"fallback", " \t"} {
		legacy["apn_mismatch_policy"] = invalid
		payload, err = json.Marshal(legacy)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(m.ProfilePath(), payload, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, ok := m.LoadProfile(); ok {
			t.Fatalf("invalid persisted policy %q silently fell back", invalid)
		}
	}
}

func hasFieldIssue(issues []FieldIssue, field string) bool {
	for _, issue := range issues {
		if issue.Field == field {
			return true
		}
	}
	return false
}

func TestEPCEnvironmentPinsEffectiveAPNMismatchPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, requested, inherited, want string
	}{
		{"default defeats inherited restricted", "", APNMismatchRestricted, APNMismatchStrict},
		{"explicit restricted defeats inherited strict", APNMismatchRestricted, APNMismatchStrict, APNMismatchRestricted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := epcEnvironment([]string{
				strings.ToLower(apnMismatchPolicyEnv) + "=" + tc.inherited,
				"LTE_UE_SNAPSHOT_PATH=old",
				"UNCHANGED=yes",
			}, "snapshot", "run", tc.requested)
			seen := 0
			for _, item := range env {
				if strings.HasPrefix(item, apnMismatchPolicyEnv+"=") {
					seen++
					if item != apnMismatchPolicyEnv+"="+tc.want {
						t.Fatalf("unexpected policy environment: %q", item)
					}
				}
			}
			if seen != 1 {
				t.Fatalf("policy environment count = %d: %v", seen, env)
			}
		})
	}
}

func TestStatusReportsEffectiveAPNMismatchPolicy(t *testing.T) {
	helper := copyHelper(t, "lte-policy-helper")
	m := New(config.Default())
	m.epcCmd = startHelper(t, helper, "sleep")
	m.enbCmd = startHelper(t, helper, "sleep")
	m.startedAt = time.Now()
	m.lastStart = validAPNPolicyParams()
	if got := m.IsRunning().APNMismatchPolicy; got != APNMismatchStrict {
		t.Fatalf("status policy = %q, want strict", got)
	}
	m.lastStart.APNMismatchPolicy = APNMismatchRestricted
	if got := m.IsRunning().APNMismatchPolicy; got != APNMismatchRestricted {
		t.Fatalf("status policy = %q, want restricted", got)
	}
}
