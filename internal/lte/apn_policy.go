package lte

import (
	"fmt"
	"strings"
)

const (
	APNMismatchStrict     = "strict"
	APNMismatchRestricted = "restricted"

	apnMismatchPolicyEnv = "LTE_APN_MISMATCH_POLICY"
)

func effectiveAPNMismatchPolicy(policy string) string {
	if policy == "" {
		return APNMismatchStrict
	}
	return policy
}

func normalizeAPNMismatchPolicy(p *StartParams) {
	p.APNMismatchPolicy = effectiveAPNMismatchPolicy(p.APNMismatchPolicy)
}

func validateAPNMismatchPolicy(policy string) error {
	effective := effectiveAPNMismatchPolicy(policy)
	if effective != APNMismatchStrict && effective != APNMismatchRestricted {
		return fmt.Errorf("must be strict or restricted")
	}
	return nil
}

// epcEnvironment pins the APN authorization mode for each EPC process. The
// inherited environment is filtered by withEnv, so a parent value can never
// override the effective request/profile policy.
func epcEnvironment(current []string, snapshotPath, runID, policy string) []string {
	filtered := make([]string, 0, len(current))
	for _, item := range current {
		key, _, ok := strings.Cut(item, "=")
		if ok && strings.EqualFold(key, apnMismatchPolicyEnv) {
			continue
		}
		filtered = append(filtered, item)
	}
	return withEnv(filtered,
		"LTE_UE_SNAPSHOT_PATH", snapshotPath,
		"LTE_UE_RUN_ID", runID,
		apnMismatchPolicyEnv, effectiveAPNMismatchPolicy(policy),
	)
}
