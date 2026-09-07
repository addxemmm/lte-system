package lte

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	sgiInterface        = "srs_spgw_sgi"
	iptablesWaitSeconds = "5"
)

var (
	routeTablePath  = "/proc/net/route"
	ipForwardPath   = "/proc/sys/net/ipv4/ip_forward"
	netInterfaceDir = "/sys/class/net"
)

// runNetworkCommand is shared by start preflight and diagnostics. Keeping the
// invocation in one place ensures every iptables operation waits for xtables'
// lock and makes command failure text useful to callers.
func runNetworkCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	out, err := exec.CommandContext(commandCtx, name, args...).CombinedOutput()
	if err == nil {
		return out, nil
	}
	if commandCtx.Err() != nil {
		return out, commandCtx.Err()
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		return out, err
	}
	return out, fmt.Errorf("%w: %s", err, detail)
}

func validInterfaceName(name string) bool {
	if name == "" || len(name) > 15 {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || strings.ContainsRune("_.:-", r) {
			continue
		}
		return false
	}
	return true
}

// defaultIPv4Route is read from procfs instead of probing an Internet host.
// procfs is scoped to the current network namespace, which is essential for
// bridge deployments where the correct uplink is the container's eth0 rather
// than the physical interface on the host.
type defaultIPv4Route struct {
	Interface string
	Gateway   string
	Metric    int
}

func resolveNetwork(requested string) (string, error) {
	if requested == "" {
		requested = "auto"
	}
	if requested != "auto" {
		if !validInterfaceName(requested) {
			return "", fmt.Errorf("invalid interface name %q", requested)
		}
		if err := checkUplinkInterface(requested); err != nil {
			return "", err
		}
		return requested, nil
	}
	routes, err := readDefaultIPv4Routes()
	if err != nil {
		return "", err
	}
	var rejected []string
	for _, route := range routes {
		if err := checkUplinkInterface(route.Interface); err == nil {
			return route.Interface, nil
		} else {
			rejected = append(rejected, fmt.Sprintf("%s: %v", route.Interface, err))
		}
	}
	return "", fmt.Errorf("no usable default IPv4 route in current network namespace (%s)", strings.Join(rejected, "; "))
}

func readDefaultIPv4Route() (defaultIPv4Route, error) {
	routes, err := readDefaultIPv4Routes()
	if err != nil {
		return defaultIPv4Route{}, err
	}
	return routes[0], nil
}

func readDefaultIPv4Routes() ([]defaultIPv4Route, error) {
	b, err := os.ReadFile(routeTablePath)
	if err != nil {
		return nil, fmt.Errorf("read default IPv4 route: %w", err)
	}
	var routes []defaultIPv4Route
	for lineNo, line := range strings.Split(string(b), "\n") {
		if lineNo == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 8 || fields[1] != "00000000" || fields[7] != "00000000" {
			continue
		}
		flags, flagErr := strconv.ParseUint(fields[3], 16, 32)
		metric, metricErr := strconv.Atoi(fields[6])
		if flagErr != nil || metricErr != nil || flags&0x1 == 0 || !validInterfaceName(fields[0]) {
			continue
		}
		gateway, gatewayErr := procHexIPv4(fields[2])
		if gatewayErr != nil {
			continue
		}
		candidate := defaultIPv4Route{Interface: fields[0], Gateway: gateway, Metric: metric}
		routes = append(routes, candidate)
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("no default IPv4 route in current network namespace main IPv4 table")
	}
	sort.SliceStable(routes, func(i, j int) bool { return routes[i].Metric < routes[j].Metric })
	return routes, nil
}

func checkUplinkInterface(name string) error {
	if name == "lo" || name == sgiInterface {
		return fmt.Errorf("interface %q cannot be used as an uplink", name)
	}
	if err := checkIface(name); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(netInterfaceDir, name, "operstate"))
	if err != nil {
		if runtime.GOOS != "linux" && os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read interface %q operstate: %w", name, err)
	}
	state := strings.TrimSpace(string(b))
	switch state {
	case "up", "unknown":
	case "":
		return fmt.Errorf("interface %q has empty operstate", name)
	default:
		return fmt.Errorf("interface %q is %s", name, state)
	}
	flagsText, err := os.ReadFile(filepath.Join(netInterfaceDir, name, "flags"))
	if err != nil {
		if runtime.GOOS != "linux" && os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read interface %q flags: %w", name, err)
	}
	flags, err := strconv.ParseUint(strings.TrimSpace(string(flagsText)), 0, 64)
	if err != nil {
		return fmt.Errorf("parse interface %q flags: %w", name, err)
	}
	if flags&0x1 == 0 {
		return fmt.Errorf("interface %q is not administratively up", name)
	}
	return nil
}

// isMissingRule recognizes the documented iptables -C not-found outcome but
// does not turn permission, lock, backend, or syntax failures into inserts.
func isMissingRule(out []byte, err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		return false
	}
	detail := strings.ToLower(strings.TrimSpace(string(out)))
	return detail == "" || strings.Contains(detail, "does a matching rule exist in that chain") ||
		strings.Contains(detail, "bad rule")
}

func procHexIPv4(s string) (string, error) {
	if len(s) != 8 {
		return "", fmt.Errorf("invalid procfs IPv4 value %q", s)
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%d.%d.%d", b[3], b[2], b[1], b[0]), nil
}

func readIPv4Forwarding() (value string, known, enabled bool, err error) {
	b, err := os.ReadFile(ipForwardPath)
	if err != nil {
		// Unit tests run on non-Linux development hosts. Production Linux must
		// expose procfs, so only the non-Linux absence is treated as unknown.
		if runtime.GOOS != "linux" && os.IsNotExist(err) {
			return "", false, false, nil
		}
		return "", false, false, err
	}
	value = strings.TrimSpace(string(b))
	return value, true, value == "1", nil
}

func requireIPv4Forwarding() error {
	value, known, enabled, err := readIPv4Forwarding()
	if err != nil {
		return fmt.Errorf("read net.ipv4.ip_forward: %w", err)
	}
	if known && !enabled {
		return fmt.Errorf("net.ipv4.ip_forward=%q; set it to 1 in the host or bridge namespace", value)
	}
	return nil
}
