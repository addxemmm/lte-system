package lte

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/addxemmm/lte-system/internal/config"
)

func installNetworkFiles(t *testing.T, routes, forwarding string, ifaces ...string) {
	t.Helper()
	dir := t.TempDir()
	routePath := filepath.Join(dir, "route")
	forwardPath := filepath.Join(dir, "ip_forward")
	interfacePath := filepath.Join(dir, "net")
	if err := os.WriteFile(routePath, []byte(routes), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(forwardPath, []byte(forwarding), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(interfacePath, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range ifaces {
		if err := os.Mkdir(filepath.Join(interfacePath, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(interfacePath, name, "operstate"), []byte("up\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(interfacePath, name, "flags"), []byte("0x1003\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	oldRoute, oldForward, oldInterfaces, oldNetworks := routeTablePath, ipForwardPath, netInterfaceDir, interfaceNetworks
	routeTablePath, ipForwardPath, netInterfaceDir = routePath, forwardPath, interfacePath
	interfaceNetworks = func() ([]interfaceNetwork, error) { return nil, nil }
	t.Cleanup(func() {
		routeTablePath, ipForwardPath, netInterfaceDir = oldRoute, oldForward, oldInterfaces
		interfaceNetworks = oldNetworks
	})
}

func fixtureRoutes() string {
	return "Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT\n" +
		"slow0 00000000 0100000A 0003 0 0 500 00000000 0 0 0\n" +
		"eth0 00000000 0164A8C0 0003 0 0 100 00000000 0 0 0\n"
}

func TestResolveNetworkAutoUsesNamespaceDefaultRoute(t *testing.T) {
	installNetworkFiles(t, fixtureRoutes(), "1\n", "eth0", "slow0")
	got, err := resolveNetwork("auto")
	if err != nil {
		t.Fatal(err)
	}
	if got != "eth0" {
		t.Fatalf("want lowest-metric default route eth0, got %q", got)
	}
	route, err := readDefaultIPv4Route()
	if err != nil {
		t.Fatal(err)
	}
	if route.Gateway != "192.168.100.1" || route.Metric != 100 {
		t.Fatalf("bad route decode: %+v", route)
	}
}

func TestResolveNetworkAutoSkipsDownRouteAndRejectsInternalLinks(t *testing.T) {
	routes := "Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT\n" +
		"down0 00000000 0100000A 0003 0 0 10 00000000 0 0 0\n" +
		"eth0 00000000 0164A8C0 0003 0 0 100 00000000 0 0 0\n"
	installNetworkFiles(t, routes, "1\n", "down0", "eth0", "lo", sgiInterface)
	if err := os.WriteFile(filepath.Join(netInterfaceDir, "down0", "operstate"), []byte("down\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveNetwork("auto"); err != nil || got != "eth0" {
		t.Fatalf("auto should skip down lower-metric route: %q %v", got, err)
	}
	for _, internal := range []string{"lo", sgiInterface} {
		if _, err := resolveNetwork(internal); err == nil || !strings.Contains(err.Error(), "cannot be used as an uplink") {
			t.Fatalf("internal interface %q accepted: %v", internal, err)
		}
	}
}

func TestResolveNetworkExplicitAndPreflightErrors(t *testing.T) {
	installNetworkFiles(t, fixtureRoutes(), "0\n", "eth0")
	if got, err := resolveNetwork("eth0"); err != nil || got != "eth0" {
		t.Fatalf("explicit uplink failed: %q %v", got, err)
	}
	if _, err := resolveNetwork("missing0"); err == nil || !strings.Contains(err.Error(), "unknown network interface") {
		t.Fatalf("missing interface should be explicit: %v", err)
	}
	if err := requireIPv4Forwarding(); err == nil || !strings.Contains(err.Error(), "ip_forward") {
		t.Fatalf("disabled forwarding should fail before RF: %v", err)
	}
}

func TestStartRejectsDisabledForwardingBeforeChildStart(t *testing.T) {
	installNetworkFiles(t, fixtureRoutes(), "0\n", "eth0")
	helper := copyHelper(t, "lte-helper")
	marker := filepath.Join(t.TempDir(), "started")
	t.Setenv("LTE_TEST_HELPER_MODE", "sleep")
	t.Setenv("LTE_TEST_HELPER_MARKER", marker)
	cfg := testConfig(t)
	cfg.SrsEPCBin = helper
	cfg.SrsENBBin = helper
	cfg.TcpdumpBin = helper
	m := New(cfg)
	_, err := m.Start(context.Background(), StartParams{
		Band: "7", APN: "test", MCC: "001", MNC: "01", Network: "eth0", SDR: "zmq",
	})
	if err == nil || !strings.Contains(err.Error(), "ip_forward") {
		t.Fatalf("disabled forwarding must fail preflight: %v", err)
	}
	matches, globErr := filepath.Glob(marker + ".*")
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("network preflight failure started child processes: %v", matches)
	}
	if m.Stop() {
		t.Fatal("failed preflight must leave no owned lifecycle")
	}
}

func TestNetworkDiagnosticsIsConfigurationEvidence(t *testing.T) {
	installNetworkFiles(t, fixtureRoutes(), "1\n", "eth0", sgiInterface)
	iptables := copyHelper(t, "iptables")
	t.Setenv("PATH", filepath.Dir(iptables)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LTE_TEST_IPTABLES", "present")

	m := New(testConfig(t))
	m.lastStart.Network = "auto"
	m.lastNetwork = "eth0"
	m.natOwned = true
	m.forwardingOwned = append([]iptRule(nil), forwardRules("eth0")...)
	d := m.NetworkDiagnostics(context.Background())
	if d.State != "configured" || d.Evidence != "configuration_only" {
		t.Fatalf("unexpected diagnostic state: %+v", d)
	}
	if d.RequestedNetwork != "auto" || d.ResolvedNetwork != "eth0" || !d.DefaultRoute.Present {
		t.Fatalf("network policy was not preserved/resolved separately: %+v", d)
	}
	if !d.SGI.Present || !d.IPv4Forward.Enabled {
		t.Fatalf("missing namespace evidence: %+v", d)
	}
	if len(d.Rules.NAT) != 1 || len(d.Rules.Filter) != 3 || len(d.Rules.Mangle) != 2 {
		t.Fatalf("unexpected rule diagnostics: %+v", d.Rules)
	}
	for _, rule := range append(append(d.Rules.NAT, d.Rules.Filter...), d.Rules.Mangle...) {
		if !rule.Present || !rule.Owned {
			t.Fatalf("owned rule not reported: %+v", rule)
		}
	}
	if len(d.Limitations) < 2 || !strings.Contains(d.Limitations[1], "does not prove Internet") {
		t.Fatalf("configuration evidence must not claim connectivity: %+v", d.Limitations)
	}
}

func TestNetworkDiagnosticsReportsMissingActiveRulesWithoutRepair(t *testing.T) {
	installNetworkFiles(t, fixtureRoutes(), "1\n", "eth0", sgiInterface)
	iptables := copyHelper(t, "iptables")
	logPath := filepath.Join(t.TempDir(), "iptables.log")
	t.Setenv("PATH", filepath.Dir(iptables)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LTE_TEST_IPTABLES", "absent")
	t.Setenv("LTE_TEST_IPTABLES_LOG", logPath)

	m := New(testConfig(t))
	m.lastStart.Network = "auto"
	m.lastNetwork = "eth0"
	m.natOwned = true
	d := m.NetworkDiagnostics(context.Background())
	if d.State != "degraded" || len(d.Problems) < 5 {
		t.Fatalf("missing active rules should be degraded: %+v", d)
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), " -I ") || strings.Contains(string(b), " -A ") || strings.Contains(string(b), " -D ") {
		t.Fatalf("diagnostics mutated firewall rules:\n%s", b)
	}
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.ConfDir = filepath.Join(cfg.DataDir, "conf")
	cfg.LogDir = filepath.Join(cfg.DataDir, "log")
	return cfg
}

func TestForwardingFailureReturnsPartialOwnershipForRollback(t *testing.T) {
	iptables := copyHelper(t, "iptables")
	logPath := filepath.Join(t.TempDir(), "iptables.log")
	t.Setenv("PATH", filepath.Dir(iptables)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LTE_TEST_IPTABLES", "absent")
	t.Setenv("LTE_TEST_IPTABLES_LOG", logPath)
	t.Setenv("LTE_TEST_IPTABLES_FAIL_CONTAINS", "--ctstate")

	owned, err := ensureForwarding(context.Background(), "eth0")
	if err == nil {
		t.Fatal("expected the second forwarding insertion to fail")
	}
	if len(owned) != 1 {
		t.Fatalf("want first inserted rule returned for rollback, got %d", len(owned))
	}
	cleanupForwarding(owned)
	b, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(b), "-D FORWARD -i srs_spgw_sgi") {
		t.Fatalf("partial insertion was not removed:\n%s", b)
	}
}

func TestEnsureRuleDoesNotInsertAfterCheckFailure(t *testing.T) {
	iptables := copyHelper(t, "iptables")
	logPath := filepath.Join(t.TempDir(), "iptables.log")
	t.Setenv("PATH", filepath.Dir(iptables)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LTE_TEST_IPTABLES", "checkfail")
	t.Setenv("LTE_TEST_IPTABLES_LOG", logPath)
	if owned, err := ensureRule(context.Background(), natRule("eth0")); err == nil || owned {
		t.Fatalf("operational -C failure must abort without insertion: owned=%v err=%v", owned, err)
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), " -I ") {
		t.Fatalf("check failure was mistaken for an absent rule:\n%s", b)
	}
}

func TestStopRetainsRuleOwnershipWhenDeletionFails(t *testing.T) {
	iptables := copyHelper(t, "iptables")
	t.Setenv("PATH", filepath.Dir(iptables)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LTE_TEST_IPTABLES", "present")
	t.Setenv("LTE_TEST_IPTABLES_FAIL_DELETE_CONTAINS", "srs_spgw_sgi")
	m := New(testConfig(t))
	m.lastNetwork = "eth0"
	m.forwardingOwned = []iptRule{forwardRules("eth0")[0]}
	if m.Stop() {
		t.Fatal("failed deletion must make Stop incomplete")
	}
	if len(m.forwardingOwned) != 1 || m.lastNetwork != "eth0" {
		t.Fatalf("failed deletion lost retry ownership: %+v %q", m.forwardingOwned, m.lastNetwork)
	}
	if _, err := m.Start(context.Background(), StartParams{
		Band: "7", APN: "test", MCC: "001", MNC: "01", Network: "eth0", SDR: "zmq",
	}); err == nil || !strings.Contains(err.Error(), "could not be cleaned up") {
		t.Fatalf("Start must not overwrite stale cleanup ownership: %v", err)
	}
	if len(m.forwardingOwned) != 1 {
		t.Fatal("failed Start lost stale cleanup ownership")
	}
	t.Setenv("LTE_TEST_IPTABLES_FAIL_DELETE_CONTAINS", "")
	if !m.Stop() || len(m.forwardingOwned) != 0 || m.lastNetwork != "" {
		t.Fatal("second Stop should retry and finish cleanup")
	}
}
