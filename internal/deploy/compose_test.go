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

type composeService struct {
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
}

type composeSpec struct {
	Services map[string]composeService `yaml:"services"`
	Volumes  map[string]any
	Networks map[string]struct {
		Driver string
		IPAM   struct{ Config []struct{ Subnet string } }
	}
}

func deploymentPath(parts ...string) string {
	return filepath.Join(append([]string{"..", ".."}, parts...)...)
}

func readCompose(t *testing.T, name string) composeSpec {
	t.Helper()
	b, err := os.ReadFile(deploymentPath("deploy", "docker", name))
	if err != nil {
		t.Fatal(err)
	}
	var spec composeSpec
	if err := yaml.Unmarshal(b, &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestComposeListenerContract(t *testing.T) {
	host := readCompose(t, "docker-compose.yml")
	bridge := readCompose(t, "docker-compose.bridge.yml")
	testOverride := readCompose(t, "docker-compose.test.yml")
	h, b := host.Services["lte-system"], bridge.Services["lte-system"]
	test := testOverride.Services["lte-system"]

	if h.NetworkMode != "host" || len(h.Ports) != 0 || b.NetworkMode != "" {
		t.Fatal("host rollback and isolated bridge publishing must remain separate")
	}
	if !reflect.DeepEqual(b.Ports, []string{"${LTE_UI_PORT:-8080}:8080"}) {
		t.Fatalf("default bridge must publish only the Web UI, got %v", b.Ports)
	}
	if b.Environment["LTE_UI_LISTEN"] != "0.0.0.0:8080" ||
		b.Environment["LTE_LISTEN"] != "0.0.0.0:8081" ||
		b.Environment["LTE_EXPOSE_API"] != "false" {
		t.Fatalf("unexpected bridge listener environment: %v", b.Environment)
	}
	if h.Environment["LTE_UI_LISTEN"] != "${LTE_UI_LISTEN:-0.0.0.0:8080}" ||
		h.Environment["LTE_LISTEN"] != "${LTE_LISTEN:-0.0.0.0:8081}" ||
		h.Environment["LTE_EXPOSE_API"] != "${LTE_EXPOSE_API:-false}" {
		t.Fatalf("unexpected host listener environment: %v", h.Environment)
	}
	if test.Environment["LTE_EXPOSE_API"] != "true" ||
		!reflect.DeepEqual(test.Ports, []string{"${LTE_API_PORT:-8081}:8081"}) {
		t.Fatalf("test override must explicitly enable and publish the direct API: %+v", test)
	}
	mergedPorts := append(append([]string{}, b.Ports...), test.Ports...)
	if !reflect.DeepEqual(mergedPorts, []string{"${LTE_UI_PORT:-8080}:8080", "${LTE_API_PORT:-8081}:8081"}) {
		t.Fatalf("bridge plus test override must retain both ports, got %v", mergedPorts)
	}
	if b.Environment["LTE_API_TOKEN"] != "${LTE_API_TOKEN:-}" || h.Environment["LTE_API_TOKEN"] != "${LTE_API_TOKEN:-}" {
		t.Fatal("existing token forwarding must be preserved on both base deployments")
	}
}

func TestBridgeIsolationAndHardwarePreservation(t *testing.T) {
	host := readCompose(t, "docker-compose.yml")
	bridge := readCompose(t, "docker-compose.bridge.yml")
	h, b := host.Services["lte-system"], bridge.Services["lte-system"]

	if len(b.Networks) != 1 || b.Networks["lte-uplink"].InterfaceName != "eth0" || bridge.Networks["lte-uplink"].Driver != "bridge" {
		t.Fatal("bridge must have exactly one fixed eth0 uplink")
	}
	if b.Sysctls["net.ipv4.ip_forward"] != "1" {
		t.Fatal("bridge must enable namespaced forwarding")
	}
	if !reflect.DeepEqual(h.Volumes, b.Volumes) || !reflect.DeepEqual(host.Volumes, bridge.Volumes) {
		t.Fatal("network migration must keep USB and the same logical data volume")
	}
	if !contains(h.Volumes, "/dev/bus/usb:/dev/bus/usb") || !contains(h.Volumes, "lte-data:/data") {
		t.Fatal("USB passthrough and persistent LTE data volume are required")
	}
	if !b.Privileged || b.Restart != "no" || b.Environment["UHD_FPGA"] != "compat" {
		t.Fatal("hardware compatibility or manual-start contract changed")
	}
	if b.StopGracePeriod != "210s" || b.StopGracePeriod != h.StopGracePeriod {
		t.Fatal("SIM-safe shutdown grace period must be preserved")
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

func TestDockerfilesUseEmbeddedUIWithoutNodeOrNginx(t *testing.T) {
	for _, name := range []string{"Dockerfile", "Dockerfile.web-upgrade"} {
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile(deploymentPath("deploy", "docker", name))
			if err != nil {
				t.Fatal(err)
			}
			text := string(b)
			lower := strings.ToLower(text)
			if !strings.Contains(text, "COPY internal/ ./internal/") {
				t.Fatal("go-builder must copy internal/, including go:embed UI assets")
			}
			for _, forbidden := range []string{"from node:", "npm ", "from nginx", "apt-get install nginx"} {
				if strings.Contains(lower, forbidden) {
					t.Fatalf("Dockerfile must not add a Node/nginx frontend runtime: found %q", forbidden)
				}
			}
		})
	}

	b, err := os.ReadFile(deploymentPath("deploy", "docker", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	var expose []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "EXPOSE ") {
			expose = append(expose, line)
		}
	}
	if !reflect.DeepEqual(expose, []string{"EXPOSE 8080"}) {
		t.Fatalf("canonical image metadata must expose only the default UI port, got %v", expose)
	}
	if !strings.Contains(string(b), "COPY deploy/docker/*.yml deploy/docker/Dockerfile* ./deploy/docker/") {
		t.Fatal("canonical go-builder must copy every deployment contract used by internal/deploy tests")
	}
}

func TestWebUpgradeRequiresVerifiedBaseAndOnlyReplacesAppLayer(t *testing.T) {
	b, err := os.ReadFile(deploymentPath("deploy", "docker", "Dockerfile.web-upgrade"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, required := range []string{
		"ARG RUNTIME_BASE\n", "FROM golang:1.26.8-bookworm AS go-builder", "FROM ${RUNTIME_BASE}",
		"ARG OCI_VERSION=2.1", "ARG OCI_REVISION=unknown",
		"COPY --from=go-builder /out/lte-system /usr/local/bin/lte-system",
		"COPY configs/app.yaml.example /app/configs/app.yaml",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("web-upgrade contract missing %q", required)
		}
	}
	for _, forbidden := range []string{"LTE_API_TOKEN", "third_party/", "firmware/", "srs-builder"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("web-upgrade must preserve the base core and never accept a token: found %q", forbidden)
		}
	}
	var runtimeBaseArgs []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ARG RUNTIME_BASE") {
			runtimeBaseArgs = append(runtimeBaseArgs, line)
		}
	}
	if len(runtimeBaseArgs) == 0 || runtimeBaseArgs[0] != "ARG RUNTIME_BASE" {
		t.Fatalf("RUNTIME_BASE must be declared without a default, got %v", runtimeBaseArgs)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
