package deploy

import (
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type composeSpec struct {
	Services map[string]struct {
		NetworkMode     string `yaml:"network_mode"`
		Privileged      bool
		Restart         string
		StopGracePeriod string `yaml:"stop_grace_period"`
		Ports           []string
		Volumes         []string
		Environment     map[string]string
		Sysctls         map[string]string
		Networks        map[string]struct {
			InterfaceName string `yaml:"interface_name"`
		}
	} `yaml:"services"`
	Volumes  map[string]any
	Networks map[string]struct {
		Driver string
		IPAM   struct{ Config []struct{ Subnet string } }
	}
}

func readCompose(t *testing.T, name string) composeSpec {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "deploy", "docker", name))
	if err != nil {
		t.Fatal(err)
	}
	var spec composeSpec
	if err := yaml.Unmarshal(b, &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestBridgeIsolationAndHardwarePreservation(t *testing.T) {
	host := readCompose(t, "docker-compose.yml")
	bridge := readCompose(t, "docker-compose.bridge.yml")
	h, b := host.Services["lte-system"], bridge.Services["lte-system"]
	if h.NetworkMode != "host" || b.NetworkMode != "" {
		t.Fatal("host rollback and isolated bridge must remain separate")
	}
	if len(b.Networks) != 1 || b.Networks["lte-uplink"].InterfaceName != "eth0" || bridge.Networks["lte-uplink"].Driver != "bridge" {
		t.Fatal("bridge must have exactly one fixed eth0 uplink")
	}
	if b.Sysctls["net.ipv4.ip_forward"] != "1" {
		t.Fatal("bridge must enable namespaced forwarding")
	}
	if len(b.Ports) != 1 || b.Ports[0] != "${LTE_BIND_ADDR:-0.0.0.0}:${LTE_API_PORT:-8081}:8081/tcp" {
		t.Fatal("only the all-interface API port should be published")
	}
	if !reflect.DeepEqual(h.Volumes, b.Volumes) || !reflect.DeepEqual(host.Volumes, bridge.Volumes) {
		t.Fatal("network migration must keep USB and the same logical data volume")
	}
	if !b.Privileged || b.Restart != "no" || b.Environment["UHD_FPGA"] != "compat" || b.Environment["LTE_LISTEN"] != "0.0.0.0:8081" {
		t.Fatal("hardware/listener/manual-start contract changed")
	}
	if b.StopGracePeriod != "210s" || b.StopGracePeriod != h.StopGracePeriod || b.Environment["LTE_API_TOKEN"] != "${LTE_API_TOKEN:-}" {
		t.Fatal("graceful shutdown and existing authentication must be preserved")
	}
	ipam := bridge.Networks["lte-uplink"].IPAM.Config
	if len(ipam) != 1 || ipam[0].Subnet != "${LTE_BRIDGE_SUBNET:-172.30.8.0/24}" {
		t.Fatal("bridge requires an explicit, overridable subnet")
	}
	defaultSubnet := strings.TrimSuffix(strings.TrimPrefix(ipam[0].Subnet, "${LTE_BRIDGE_SUBNET:-"), "}")
	prefix, err := netip.ParsePrefix(defaultSubnet)
	if err != nil || prefix.Overlaps(netip.MustParsePrefix("172.16.0.0/24")) {
		t.Fatal("default bridge must not overlap the UE pool")
	}
	for k, v := range b.Environment {
		if strings.Contains(strings.ToLower(k), "autostart") && v != "" {
			t.Fatal("bridge must not start RF automatically")
		}
	}
}
