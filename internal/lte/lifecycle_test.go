package lte

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/addxemmm/lte-system/internal/config"
)

// TestMain also makes a copied lte.test binary a deterministic short-lived
// process fixture. It never starts RF, packet capture, or hashcat.
func TestMain(m *testing.M) {
	base := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	if base == "ps" && os.Getenv("LTE_TEST_PS") != "" {
		_, _ = os.Stdout.WriteString(os.Getenv("LTE_TEST_PS"))
		os.Exit(0)
	}
	if mode := os.Getenv("LTE_TEST_IPTABLES"); base == "iptables" && mode != "" {
		f, _ := os.OpenFile(os.Getenv("LTE_TEST_IPTABLES_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if f != nil {
			_, _ = f.WriteString(strings.Join(os.Args[1:], " ") + "\n")
			_ = f.Close()
		}
		if mode == "absent" && slices.Contains(os.Args[1:], "-C") {
			os.Exit(1)
		}
		if mode == "checkfail" && slices.Contains(os.Args[1:], "-C") {
			os.Exit(2)
		}
		if fail := os.Getenv("LTE_TEST_IPTABLES_FAIL_CONTAINS"); fail != "" &&
			slices.Contains(os.Args[1:], "-I") && strings.Contains(strings.Join(os.Args[1:], " "), fail) {
			os.Exit(2)
		}
		if fail := os.Getenv("LTE_TEST_IPTABLES_FAIL_DELETE_CONTAINS"); fail != "" &&
			slices.Contains(os.Args[1:], "-D") && strings.Contains(strings.Join(os.Args[1:], " "), fail) {
			os.Exit(2)
		}
		os.Exit(0)
	}
	switch os.Getenv("LTE_TEST_HELPER_MODE") {
	case "exit0":
		os.Exit(0)
	case "exit1":
		os.Exit(7)
	case "sleep":
		if logPath := os.Getenv("LTE_TEST_ENV_LOG"); logPath != "" && base == "srsepc" {
			_ = os.WriteFile(logPath, []byte(os.Getenv("LTE_UE_SNAPSHOT_PATH")+"\n"+os.Getenv("LTE_UE_RUN_ID")+"\n"), 0o644)
		}
		if marker := os.Getenv("LTE_TEST_HELPER_MARKER"); marker != "" {
			_ = os.WriteFile(marker+"."+filepath.Base(os.Args[0]), []byte("started"), 0o644)
		}
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func copyHelper(t *testing.T, name string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dst := filepath.Join(t.TempDir(), name)
	b, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil {
		t.Fatal(err)
	}
	return dst
}

func startHelper(t *testing.T, path, mode string) *managedChild {
	t.Helper()
	cmd := exec.Command(path)
	cmd.Env = append(os.Environ(), "LTE_TEST_HELPER_MODE="+mode)
	child, err := startManaged(cmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopManaged(child, 2*time.Second) })
	return child
}

func waitDead(t *testing.T, child *managedChild) {
	t.Helper()
	select {
	case <-child.done:
	case <-time.After(3 * time.Second):
		t.Fatal("helper did not exit")
	}
	if alive(child) {
		t.Fatal("completed Wait must be reported stopped")
	}
}

func TestManagedChildExitStates(t *testing.T) {
	helper := copyHelper(t, "lte-helper")
	t.Run("normal", func(t *testing.T) {
		waitDead(t, startHelper(t, helper, "exit0"))
	})
	t.Run("nonzero", func(t *testing.T) {
		waitDead(t, startHelper(t, helper, "exit1"))
	})
	t.Run("signal", func(t *testing.T) {
		child := startHelper(t, helper, "sleep")
		stopManaged(child, 2*time.Second)
		waitDead(t, child)
	})
}

func TestStatusAndStopConcurrent(t *testing.T) {
	helper := copyHelper(t, "lte-helper")
	m := New(config.Default())
	m.epcCmd = startHelper(t, helper, "sleep")
	m.enbCmd = startHelper(t, helper, "sleep")
	m.pcapCmd = startHelper(t, helper, "sleep")
	m.startedAt = time.Now()

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 20; j++ {
				_ = m.IsRunning()
			}
		}()
	}
	close(start)
	if !m.Stop() {
		t.Fatal("owned children should be stopped")
	}
	wg.Wait()
	if st := m.IsRunning(); st.Running || st.EPC || st.ENB || st.Pcap {
		t.Fatalf("unexpected final status: %+v", st)
	}
}

func TestStopDoesNotKillUnownedProcess(t *testing.T) {
	foreign := startHelper(t, copyHelper(t, "tcpdump"), "sleep")
	m := New(config.Default())
	if m.Stop() {
		t.Fatal("idle manager should report no owned lifecycle")
	}
	if !alive(foreign) {
		t.Fatal("Stop killed an unowned same-name process")
	}
	if st := m.IsRunning(); st.Pcap || st.Running {
		t.Fatalf("unowned process leaked into managed status: %+v", st)
	}
}

func TestStopCleansOnlyRecordedNetworkRules(t *testing.T) {
	dir := t.TempDir()
	iptables := copyHelper(t, "iptables")
	logPath := filepath.Join(dir, "iptables.log")
	t.Setenv("PATH", filepath.Dir(iptables)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LTE_TEST_IPTABLES", "1")
	t.Setenv("LTE_TEST_IPTABLES_LOG", logPath)

	m := New(config.Default())
	// Model one NAT and one forwarding rule that this Manager added. Other
	// rules are intentionally absent from the ownership record.
	m.lastNetwork = "TARGET"
	m.natOwned = true
	m.forwardingOwned = []iptRule{forwardRules("")[0]}
	if !m.Stop() {
		t.Fatal("owned network resources should count as a stopped lifecycle")
	}
	if m.Stop() {
		t.Fatal("second idle Stop should do nothing")
	}
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.FieldsFunc(strings.TrimSpace(string(b)), func(r rune) bool { return r == '\n' || r == '\r' })
	if len(lines) != 2 {
		t.Fatalf("want exactly two owned rule deletions, got %d:\n%s", len(lines), b)
	}
}

func TestStartCancellationRollsBack(t *testing.T) {
	installNetworkFiles(t, fixtureRoutes(), "1\n", "eth0", "slow0")
	epcHelper := copyHelper(t, "srsepc")
	enbHelper := copyHelper(t, "srsenb")
	tcpdumpHelper := copyHelper(t, "tcpdump")
	iptables := copyHelper(t, "iptables")
	marker := filepath.Join(t.TempDir(), "started")
	iptablesLog := filepath.Join(t.TempDir(), "iptables.log")
	t.Setenv("LTE_TEST_HELPER_MODE", "sleep")
	t.Setenv("LTE_TEST_HELPER_MARKER", marker)
	t.Setenv("LTE_TEST_IPTABLES", "absent")
	t.Setenv("LTE_TEST_IPTABLES_LOG", iptablesLog)
	t.Setenv("PATH", filepath.Dir(iptables)+string(os.PathListSeparator)+os.Getenv("PATH"))

	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.ConfDir = filepath.Join(cfg.DataDir, "conf")
	cfg.LogDir = filepath.Join(cfg.DataDir, "log")
	cfg.SrsEPCBin = epcHelper
	cfg.SrsENBBin = enbHelper
	cfg.TcpdumpBin = tcpdumpHelper
	m := New(cfg)

	oldEPC, oldENB := epcInitDelay, enbInitDelay
	epcInitDelay, enbInitDelay = 20*time.Millisecond, 30*time.Second
	t.Cleanup(func() { epcInitDelay, enbInitDelay = oldEPC, oldENB })

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := m.Start(ctx, StartParams{
			Band: "7", APN: "test", MCC: "001", MNC: "01",
			Network: "eth0", SDR: "zmq",
		})
		result <- err
	}()
	enbMarker := marker + "." + filepath.Base(enbHelper)
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(enbMarker); err == nil {
			break
		}
		select {
		case err := <-result:
			cancel()
			t.Fatalf("Start returned before fake eNB init: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("fake eNB was not started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("want context cancellation, got %v", err)
	}
	if st := m.IsRunning(); st.Running || st.Pcap {
		t.Fatalf("canceled start leaked children: %+v", st)
	}
	if m.Stop() {
		t.Fatal("rollback should leave Manager idle")
	}
	b, err := os.ReadFile(iptablesLog)
	if err != nil {
		t.Fatal(err)
	}
	deletes := 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.Contains(" "+line+" ", " -D ") {
			deletes++
		}
	}
	if deletes != 6 {
		t.Fatalf("rollback should delete its NAT and 5 forwarding rules; got %d:\n%s", deletes, b)
	}
}

func TestTailFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.log")
	content := "l1\n\nl2\nl3\nl4\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := tailFile(p, 2); got != "l3 | l4" {
		t.Fatalf("want last 2 lines, got %q", got)
	}
	if got := tailFile(p, 99); got != "l1 | l2 | l3 | l4" {
		t.Fatalf("want all lines, got %q", got)
	}
	if got := tailFile(filepath.Join(t.TempDir(), "missing"), 2); got == "" {
		t.Fatal("missing file should return error text, not empty")
	}
}
