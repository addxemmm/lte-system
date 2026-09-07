package lte

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUENetworkPlanValidation(t *testing.T) {
	plan, err := makeUENetworkPlan("10.20.30.0/24", "isolated")
	if err != nil || plan.SGIAddress != "10.20.30.1" {
		t.Fatalf("valid private /24 rejected: %+v %v", plan, err)
	}
	for _, tc := range []struct{ subnet, access string }{
		{"8.8.8.0/24", "isolated"}, {"10.20.30.1/24", "isolated"},
		{"10.20.30.0/25", "isolated"}, {"10.20.30.0/24", "groups"},
	} {
		if _, err := makeUENetworkPlan(tc.subnet, tc.access); err == nil {
			t.Fatalf("invalid UE plan accepted: %+v", tc)
		}
	}
}

func TestAPNValidationBoundaries(t *testing.T) {
	valid99 := strings.Repeat("a", 63) + "." + strings.Repeat("b", 35)
	for _, value := range []string{"internet", "corp-lab.example", valid99} {
		if err := validateAPN(value); err != nil {
			t.Fatalf("valid APN %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"", "-bad", "bad-", "bad..label", "bad_label",
		strings.Repeat("a", 64), valid99 + "c"} {
		if err := validateAPN(value); err == nil {
			t.Fatalf("invalid APN %q accepted", value)
		}
	}
}

func TestUENetworkStaticAddressesAndNamespaceConflicts(t *testing.T) {
	cfg := testConfig(t)
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	plan, _ := makeUENetworkPlan("10.20.30.0/24", "isolated")
	m := New(cfg)
	oldNetworks := interfaceNetworks
	interfaceNetworks = func() ([]interfaceNetwork, error) { return nil, nil }
	t.Cleanup(func() { interfaceNetworks = oldNetworks })
	row := func(ip string) string {
		return "ue0,mil,001010123456780,00112233445566778899aabbccddeeff,opc," +
			"63bfa50ee6523365ff14c1f45f88737d,8001,000000003521,7," + ip + "\n"
	}
	writeDB := func(ip string) {
		t.Helper()
		if err := os.WriteFile(cfg.UserDBPath(), []byte(row(ip)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeDB("10.20.30.2")
	if err := m.validateUENetworkEnvironment(plan); err != nil {
		t.Fatalf("usable static address rejected: %v", err)
	}
	for _, ip := range []string{"10.20.30.0", "10.20.30.1", "10.20.30.255", "10.20.31.2"} {
		writeDB(ip)
		if err := m.validateUENetworkEnvironment(plan); err == nil {
			t.Fatalf("invalid static address %s accepted", ip)
		}
	}
	writeDB("dynamic")
	interfaceNetworks = func() ([]interfaceNetwork, error) {
		return []interfaceNetwork{{Name: "docker_lte-uplink", Prefix: netip.MustParsePrefix("10.20.30.0/24")}}, nil
	}
	if err := m.validateUENetworkEnvironment(plan); err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Fatalf("namespace subnet conflict not rejected: %v", err)
	}
}

func TestForwardRulesParameterizeSubnetAndUEAccess(t *testing.T) {
	isolated, _ := makeUENetworkPlan("10.20.30.0/24", "isolated")
	allow, _ := makeUENetworkPlan("10.20.30.0/24", "allow")
	isolatedRules := forwardRules("eth0", isolated)
	allowRules := forwardRules("eth0", allow)
	if len(isolatedRules) != 5 || isolatedRules[4].args[len(isolatedRules[4].args)-1] != "DROP" {
		t.Fatalf("bad isolation policy: %+v", isolatedRules)
	}
	if allowRules[4].args[len(allowRules[4].args)-1] != "ACCEPT" {
		t.Fatalf("bad allow policy: %+v", allowRules[4])
	}
	for _, rule := range isolatedRules {
		if strings.Contains(strings.Join(rule.args, " "), defaultUESubnet) {
			t.Fatalf("custom plan leaked default subnet: %+v", rule)
		}
	}
	if got := strings.Join(natRule("eth0", isolated.SubnetText).args, " "); !strings.Contains(got, "10.20.30.0/24") {
		t.Fatalf("NAT was not parameterized: %s", got)
	}
}

func TestEnsureForwardingInstallsUEPolicyAtHead(t *testing.T) {
	iptables := copyHelper(t, "iptables")
	logPath := filepath.Join(t.TempDir(), "iptables.log")
	t.Setenv("PATH", filepath.Dir(iptables)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LTE_TEST_IPTABLES", "absent")
	t.Setenv("LTE_TEST_IPTABLES_LOG", logPath)
	plan, _ := makeUENetworkPlan("10.20.30.0/24", "isolated")
	owned, err := ensureForwarding(context.Background(), "eth0", plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 5 {
		t.Fatalf("want all five rules owned, got %d", len(owned))
	}
	t.Cleanup(func() { cleanupForwarding(owned) })
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var inserts []string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.Contains(" "+line+" ", " -I ") {
			inserts = append(inserts, line)
		}
	}
	if len(inserts) != 5 || !strings.Contains(inserts[4], "-i srs_spgw_sgi -o srs_spgw_sgi") ||
		!strings.HasSuffix(inserts[4], "-j DROP") {
		t.Fatalf("UE policy must be the final -I and therefore chain head:\n%s", b)
	}
}

func TestRunIDFailureIsFailClosed(t *testing.T) {
	installNetworkFiles(t, fixtureRoutes(), "1\n", "eth0", "slow0")
	cfg := testConfig(t)
	oldGenerator := generateRunID
	generateRunID = func() (string, error) { return "", errors.New("entropy unavailable") }
	t.Cleanup(func() { generateRunID = oldGenerator })
	m := New(cfg)
	_, err := m.Start(context.Background(), StartParams{
		Band: "7", APN: "test", MCC: "001", MNC: "01", Network: "eth0", SDR: "zmq",
	})
	if err == nil || !strings.Contains(err.Error(), "run ID") {
		t.Fatalf("run ID failure did not fail closed: %v", err)
	}
	if m.hasLifecycleLocked() || m.UESource().Running {
		t.Fatal("run ID failure created an owned lifecycle or telemetry source")
	}
}

func TestStartPublishesOwnedUESourceAndRemovesStaleSnapshot(t *testing.T) {
	installNetworkFiles(t, fixtureRoutes(), "1\n", "eth0", "slow0")
	epcHelper := copyHelper(t, "srsepc")
	enbHelper := copyHelper(t, "srsenb")
	tcpdumpHelper := copyHelper(t, "tcpdump")
	iptables := copyHelper(t, "iptables")
	t.Setenv("PATH", filepath.Dir(iptables)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LTE_TEST_IPTABLES", "absent")
	t.Setenv("LTE_TEST_HELPER_MODE", "sleep")
	envLog := filepath.Join(t.TempDir(), "epc-env.log")
	t.Setenv("LTE_TEST_ENV_LOG", envLog)
	cfg := testConfig(t)
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	cfg.SrsEPCBin, cfg.SrsENBBin, cfg.TcpdumpBin = epcHelper, enbHelper, tcpdumpHelper
	snapshot := cfg.LogPath("ue-sessions.json")
	if err := os.WriteFile(snapshot, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldEPC, oldENB := epcInitDelay, enbInitDelay
	epcInitDelay, enbInitDelay = 20*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { epcInitDelay, enbInitDelay = oldEPC, oldENB })
	m := New(cfg)
	if _, err := m.Start(context.Background(), StartParams{
		Band: "7", APN: "test", MCC: "001", MNC: "01", Network: "eth0", SDR: "zmq",
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Stop() })
	source := m.UESource()
	if !source.Running || len(source.RunID) != 32 || source.PID <= 0 || source.Path != snapshot {
		t.Fatalf("bad UE source: %+v", source)
	}
	if _, err := os.Stat(snapshot); !os.IsNotExist(err) {
		t.Fatalf("stale snapshot was not removed before EPC launch: %v", err)
	}
	envBytes, err := os.ReadFile(envLog)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(envBytes); !strings.Contains(got, snapshot+"\n"+source.RunID+"\n") {
		t.Fatalf("EPC environment does not match owned source: %q", got)
	}
	plan := m.NetworkPlanSnapshot()
	if !plan.Active || plan.UESubnet != defaultUESubnet || plan.UEAccess != defaultUEAccess {
		t.Fatalf("bad effective network plan: %+v", plan)
	}
	if !m.Stop() || m.UESource().RunID != "" {
		t.Fatal("Stop did not clear UE source metadata")
	}
}
