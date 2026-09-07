//go:build linux

package lte

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

const (
	kernelNetworkTestEnv = "LTE_KERNEL_NETWORK_TEST"
	tunSetIFF            = 0x400454ca
	iffTun               = 0x0001
	iffNoPI              = 0x1000
)

// TestKernelUENetworkPolicy runs only in a disposable network namespace with
// NET_ADMIN and /dev/net/tun. It injects the same inner IPv4 packet that SPGW
// writes to SGi and verifies the production firewall transaction in-kernel.
func TestKernelUENetworkPolicy(t *testing.T) {
	if os.Getenv(kernelNetworkTestEnv) != "1" {
		t.Skip("set LTE_KERNEL_NETWORK_TEST=1 inside the documented disposable network-none container")
	}
	assertDisposableNetworkNamespace(t)
	if got, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward"); err != nil || strings.TrimSpace(string(got)) != "1" {
		t.Fatalf("disposable namespace must start with net.ipv4.ip_forward=1: value=%q err=%v", got, err)
	}

	policy := forwardPolicy(t)
	runKernelCommand(t, "iptables", "-w", "5", "-P", "FORWARD", "DROP")
	t.Cleanup(func() { _ = runKernelCommandErr("iptables", "-w", "5", "-P", "FORWARD", policy) })
	if got := forwardPolicy(t); got != "DROP" {
		t.Fatalf("FORWARD policy=%s, want DROP", got)
	}

	tun := createTUN(t, sgiInterface)
	t.Cleanup(func() { _ = tun.Close() })
	runKernelCommand(t, "ip", "addr", "add", "10.77.88.1/24", "dev", sgiInterface)
	runKernelCommand(t, "ip", "link", "set", "dev", sgiInterface, "up")
	setTestSysctl(t, "/proc/sys/net/ipv4/conf/all/send_redirects", "0")
	setTestSysctl(t, "/proc/sys/net/ipv4/conf/"+sgiInterface+"/send_redirects", "0")
	runKernelCommand(t, "ip", "link", "add", "lte_test_up", "type", "dummy")
	t.Cleanup(func() { _ = runKernelCommandErr("ip", "link", "del", "lte_test_up") })
	runKernelCommand(t, "ip", "addr", "add", "192.0.2.2/24", "dev", "lte_test_up")
	runKernelCommand(t, "ip", "link", "set", "dev", "lte_test_up", "up")

	for _, access := range []string{"isolated", "allow"} {
		t.Run(access, func(t *testing.T) {
			drainTUN(t, tun)
			plan, err := makeUENetworkPlan("10.77.88.0/24", access)
			if err != nil {
				t.Fatal(err)
			}
			rules := forwardRules("lte_test_up", plan)
			policyRule := rules[len(rules)-1]
			target := policyRule.args[len(policyRule.args)-1]
			oppositeTarget := "ACCEPT"
			if target == "ACCEPT" {
				oppositeTarget = "DROP"
			}
			oppositeRule := policyRuleWithTarget(policyRule, oppositeTarget)
			var owned []iptRule
			finished := false
			defer func() {
				if finished {
					return
				}
				_ = cleanupForwarding(owned)
				_ = deleteOwnedRule(policyRule)
				_ = deleteOwnedRule(oppositeRule)
			}()
			// Put an opposite rule before an exact duplicate at the tail. The
			// production transaction must still insert its policy at chain head;
			// merely finding the duplicate would leave the opposite rule effective.
			for _, rule := range []iptRule{oppositeRule, policyRule} {
				if err := rule.run(context.Background(), "-A"); err != nil {
					t.Fatalf("append pre-existing policy rule %+v: %v", rule, err)
				}
			}

			owned, err = ensureForwarding(context.Background(), "lte_test_up", plan)
			if err != nil {
				t.Fatalf("install production forwarding rules: %v", err)
			}
			if len(owned) != 5 {
				t.Fatalf("isolated namespace should own five new rules, got %d", len(owned))
			}

			assertFirstForwardRule(t, target)
			runKernelCommand(t, "iptables", "-w", "5", "-Z", "FORWARD", "1")
			marker := []byte("lte-kernel-policy-" + access)
			src := netip.MustParseAddr("10.77.88.2")
			dst := netip.MustParseAddr("10.77.88.3")
			packet := makeIPv4UDPPacket(t, src, dst, marker)
			if n, err := syscall.Write(int(tun.Fd()), packet); err != nil || n != len(packet) {
				t.Fatalf("inject SGi packet: wrote=%d want=%d err=%v", n, len(packet), err)
			}

			forwarded, observed, err := readTargetUDPPacket(tun, src, dst, marker, 1500*time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			if access == "isolated" && observed {
				t.Fatalf("isolated SGi packet escaped DROP: %x", forwarded)
			}
			if access == "allow" {
				if !observed {
					t.Fatal("allow policy did not forward SGi-to-SGi packet under FORWARD DROP")
				}
				assertForwardedUDPPacket(t, forwarded, marker)
			}
			if packets := firstForwardRulePackets(t); packets < 1 {
				t.Fatalf("chain-head %s rule counter did not observe injected packet", target)
			}

			remaining := cleanupForwarding(owned)
			if len(remaining) != 0 {
				t.Fatalf("cleanup retained rules: %+v", remaining)
			}
			if again := cleanupForwarding(remaining); len(again) != 0 {
				t.Fatalf("idempotent cleanup retained rules: %+v", again)
			}
			for _, rule := range rules[:len(rules)-1] {
				assertRuleMissing(t, rule)
			}
			assertRulePresent(t, policyRule)
			assertRulePresent(t, oppositeRule)
			if err := deleteOwnedRule(policyRule); err != nil {
				t.Fatalf("remove pre-existing exact policy rule: %v", err)
			}
			if err := deleteOwnedRule(oppositeRule); err != nil {
				t.Fatalf("remove pre-existing opposite policy rule: %v", err)
			}
			assertRuleMissing(t, policyRule)
			assertRuleMissing(t, oppositeRule)
			finished = true
		})
	}
}

func setTestSysctl(t *testing.T, path, value string) {
	t.Helper()
	original, err := os.ReadFile(path)
	if err != nil {
		t.Logf("leave %s unchanged (%v); exact UDP matcher will ignore redirects", path, err)
		return
	}
	if err := os.WriteFile(path, []byte(value+"\n"), 0); err != nil {
		t.Logf("leave %s unchanged (%v); exact UDP matcher will ignore redirects", path, err)
		return
	}
	t.Cleanup(func() { _ = os.WriteFile(path, original, 0) })
}

func policyRuleWithTarget(rule iptRule, target string) iptRule {
	cloned := iptRule{table: rule.table, chain: rule.chain, args: append([]string(nil), rule.args...)}
	cloned.args[len(cloned.args)-1] = target
	return cloned
}

func assertRulePresent(t *testing.T, rule iptRule) {
	t.Helper()
	out, err := runNetworkCommand(context.Background(), "iptables", rule.commandArgs("-C")...)
	if err != nil {
		t.Fatalf("expected rule to remain: %+v err=%v output=%q", rule, err, out)
	}
}

func assertRuleMissing(t *testing.T, rule iptRule) {
	t.Helper()
	out, err := runNetworkCommand(context.Background(), "iptables", rule.commandArgs("-C")...)
	if err == nil || !isMissingRule(out, err) {
		t.Fatalf("rule remains: %+v err=%v output=%q", rule, err, out)
	}
}

func assertDisposableNetworkNamespace(t *testing.T) {
	t.Helper()
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range ifaces {
		if iface.Name != "lo" {
			t.Fatalf("refusing kernel network test outside an empty network namespace: found interface %q", iface.Name)
		}
	}
	if len(ifaces) != 1 {
		t.Fatalf("refusing kernel network test: expected only loopback, found %+v", ifaces)
	}
}

func createTUN(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.OpenFile("/dev/net/tun", os.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatalf("open /dev/net/tun: %v", err)
	}
	var ifreq [40]byte // Linux amd64 struct ifreq.
	copy(ifreq[:15], name)
	binary.NativeEndian.PutUint16(ifreq[16:18], iffTun|iffNoPI)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), tunSetIFF, uintptr(unsafe.Pointer(&ifreq[0])))
	if errno != 0 {
		_ = f.Close()
		t.Fatalf("TUNSETIFF %s: %v", name, errno)
	}
	// Character-device descriptors cannot be driven through Go's runtime
	// netpoller on every supported kernel (Ubuntu 22.04 returns
	// internal/poll.ErrNotPollable). Keep the descriptor non-blocking and use
	// raw reads/writes below so EAGAIN is observable and can be polled.
	if err := syscall.SetNonblock(int(f.Fd()), true); err != nil {
		_ = f.Close()
		t.Fatalf("set %s non-blocking: %v", name, err)
	}
	return f
}

func forwardPolicy(t *testing.T) string {
	t.Helper()
	out := runKernelCommand(t, "iptables", "-w", "5", "-S", "FORWARD")
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "-P" && fields[1] == "FORWARD" {
			return fields[2]
		}
	}
	t.Fatalf("read FORWARD policy from %q", out)
	return "ACCEPT"
}

func assertFirstForwardRule(t *testing.T, target string) {
	t.Helper()
	out := runKernelCommand(t, "iptables", "-w", "5", "-S", "FORWARD")
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "-A FORWARD ") {
			continue
		}
		for _, want := range []string{"-i " + sgiInterface, "-o " + sgiInterface,
			"-s 10.77.88.0/24", "-d 10.77.88.0/24", "-j " + target} {
			if !strings.Contains(line, want) {
				t.Fatalf("first FORWARD rule missing %q: %s", want, line)
			}
		}
		return
	}
	t.Fatalf("no FORWARD rule found in %q", out)
}

func firstForwardRulePackets(t *testing.T) uint64 {
	t.Helper()
	out := runKernelCommand(t, "iptables", "-w", "5", "-L", "FORWARD", "1", "-v", "-n", "-x")
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if packets, err := strconv.ParseUint(fields[0], 10, 64); err == nil {
			return packets
		}
	}
	t.Fatalf("parse first FORWARD rule counter from %q", out)
	return 0
}

func makeIPv4UDPPacket(t *testing.T, src, dst netip.Addr, payload []byte) []byte {
	t.Helper()
	if !src.Is4() || !dst.Is4() {
		t.Fatal("test packet requires IPv4 addresses")
	}
	packet := make([]byte, 20+8+len(payload))
	packet[0], packet[1] = 0x45, 0
	binary.BigEndian.PutUint16(packet[2:4], uint16(len(packet)))
	binary.BigEndian.PutUint16(packet[4:6], 0x1234)
	binary.BigEndian.PutUint16(packet[6:8], 0x4000)
	packet[8], packet[9] = 64, 17
	src4, dst4 := src.As4(), dst.As4()
	copy(packet[12:16], src4[:])
	copy(packet[16:20], dst4[:])
	binary.BigEndian.PutUint16(packet[10:12], internetChecksum(packet[:20]))
	udp := packet[20:]
	binary.BigEndian.PutUint16(udp[0:2], 41000)
	binary.BigEndian.PutUint16(udp[2:4], 42000)
	binary.BigEndian.PutUint16(udp[4:6], uint16(len(udp)))
	copy(udp[8:], payload)
	pseudo := make([]byte, 12+len(udp))
	copy(pseudo[0:4], src4[:])
	copy(pseudo[4:8], dst4[:])
	pseudo[9] = 17
	binary.BigEndian.PutUint16(pseudo[10:12], uint16(len(udp)))
	copy(pseudo[12:], udp)
	checksum := internetChecksum(pseudo)
	if checksum == 0 {
		checksum = 0xffff
	}
	binary.BigEndian.PutUint16(udp[6:8], checksum)
	return packet
}

func internetChecksum(data []byte) uint16 {
	var sum uint32
	for len(data) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(data[:2]))
		data = data[2:]
	}
	if len(data) == 1 {
		sum += uint32(data[0]) << 8
	}
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}

func drainTUN(t *testing.T, tun *os.File) {
	t.Helper()
	buf := make([]byte, 65535)
	for {
		_, err := syscall.Read(int(tun.Fd()), buf)
		if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
			return
		}
		if err != nil {
			t.Fatalf("drain TUN: %v", err)
		}
	}
}

func readTargetUDPPacket(tun *os.File, src, dst netip.Addr, marker []byte, timeout time.Duration) ([]byte, bool, error) {
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 65535)
	for time.Now().Before(deadline) {
		n, err := syscall.Read(int(tun.Fd()), buf)
		if err == nil {
			packet := append([]byte(nil), buf[:n]...)
			// A same-interface route may also emit an ICMP Redirect containing a
			// copy of the injected UDP packet (and therefore its marker). Ignore
			// all such control traffic and accept only the exact outer UDP flow.
			if isTargetUDPPacket(packet, src, dst, marker) {
				return packet, true, nil
			}
			continue
		}
		if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, fmt.Errorf("read TUN: %w", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil, false, nil
}

func isTargetUDPPacket(packet []byte, src, dst netip.Addr, marker []byte) bool {
	if len(packet) < 28 || packet[0]>>4 != 4 || packet[9] != 17 || !src.Is4() || !dst.Is4() {
		return false
	}
	headerLen := int(packet[0]&0x0f) * 4
	totalLen := int(binary.BigEndian.Uint16(packet[2:4]))
	if headerLen < 20 || totalLen < headerLen+8 || totalLen > len(packet) {
		return false
	}
	src4, dst4 := src.As4(), dst.As4()
	if !bytes.Equal(packet[12:16], src4[:]) || !bytes.Equal(packet[16:20], dst4[:]) {
		return false
	}
	udp := packet[headerLen:totalLen]
	udpLen := int(binary.BigEndian.Uint16(udp[4:6]))
	if udpLen != len(udp) || udpLen < 8 || binary.BigEndian.Uint16(udp[0:2]) != 41000 || binary.BigEndian.Uint16(udp[2:4]) != 42000 {
		return false
	}
	return bytes.Equal(udp[8:], marker)
}

func assertForwardedUDPPacket(t *testing.T, packet, marker []byte) {
	t.Helper()
	if len(packet) < 28 || packet[0]>>4 != 4 || packet[0]&0x0f != 5 || packet[9] != 17 {
		t.Fatalf("not a minimal IPv4 UDP packet: %x", packet)
	}
	if packet[8] != 63 {
		t.Fatalf("forwarded TTL=%d, want 63", packet[8])
	}
	if internetChecksum(packet[:20]) != 0 {
		t.Fatal("forwarded IPv4 header checksum is invalid")
	}
	if got := netip.AddrFrom4([4]byte(packet[12:16])); got.String() != "10.77.88.2" {
		t.Fatalf("forwarded source=%s", got)
	}
	if got := netip.AddrFrom4([4]byte(packet[16:20])); got.String() != "10.77.88.3" {
		t.Fatalf("forwarded destination=%s", got)
	}
	udpLen := int(binary.BigEndian.Uint16(packet[24:26]))
	if udpLen < 8 || 20+udpLen > len(packet) || !bytes.Equal(packet[28:20+udpLen], marker) {
		t.Fatalf("forwarded UDP payload mismatch: %x", packet)
	}
	pseudo := make([]byte, 12+udpLen)
	copy(pseudo[0:4], packet[12:16])
	copy(pseudo[4:8], packet[16:20])
	pseudo[9] = 17
	binary.BigEndian.PutUint16(pseudo[10:12], uint16(udpLen))
	copy(pseudo[12:], packet[20:20+udpLen])
	if internetChecksum(pseudo) != 0 {
		t.Fatal("forwarded UDP checksum is invalid")
	}
}

func runKernelCommand(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v: %s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func runKernelCommandErr(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, out)
	}
	return nil
}
