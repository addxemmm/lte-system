package lte

import (
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/addxemmm/lte-system/internal/subscriber"
)

const (
	defaultUESubnet = "172.16.0.0/24"
	defaultUEAccess = "isolated"
)

type ueNetworkPlan struct {
	Subnet     netip.Prefix
	SubnetText string
	SGIAddress string
	Access     string
}

// NetworkPlanSnapshot is the effective plan of the last successful running
// cell. Active is configuration evidence, not proof of UE connectivity.
type NetworkPlanSnapshot struct {
	Active          bool   `json:"active"`
	UESubnet        string `json:"ue_subnet"`
	SGIAddress      string `json:"sgi_address"`
	UEAccess        string `json:"ue_access"`
	ResolvedNetwork string `json:"resolved_network,omitempty"`
}

type interfaceNetwork struct {
	Name   string
	Prefix netip.Prefix
}

var interfaceNetworks = currentInterfaceNetworks

func normalizeUEPolicy(p *StartParams) {
	p.UESubnet = strings.TrimSpace(p.UESubnet)
	if p.UESubnet == "" {
		p.UESubnet = defaultUESubnet
	}
	p.UEAccess = strings.ToLower(strings.TrimSpace(p.UEAccess))
	if p.UEAccess == "" {
		p.UEAccess = defaultUEAccess
	}
}

func makeUENetworkPlan(subnet, access string) (ueNetworkPlan, error) {
	prefix, err := netip.ParsePrefix(subnet)
	if err != nil || !prefix.Addr().Is4() || prefix.Bits() != 24 || !prefix.Addr().IsPrivate() {
		return ueNetworkPlan{}, fmt.Errorf("ue_subnet must be a canonical RFC1918 IPv4 /24")
	}
	prefix = prefix.Masked()
	if prefix.String() != subnet {
		return ueNetworkPlan{}, fmt.Errorf("ue_subnet must be canonical %s", prefix)
	}
	if access != "isolated" && access != "allow" {
		return ueNetworkPlan{}, fmt.Errorf("ue_access must be isolated or allow")
	}
	return ueNetworkPlan{
		Subnet: prefix, SubnetText: prefix.String(), SGIAddress: prefix.Addr().Next().String(), Access: access,
	}, nil
}

func validateAPN(apn string) error {
	if len(apn) == 0 || len(apn) > 99 {
		return fmt.Errorf("must be 1-99 characters")
	}
	for _, label := range strings.Split(apn, ".") {
		if len(label) == 0 || len(label) > 63 {
			return fmt.Errorf("each label must be 1-63 characters")
		}
		for i, r := range label {
			alphaNum := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
			if !alphaNum && !(r == '-' && i > 0 && i < len(label)-1) {
				return fmt.Errorf("labels must contain only letters, digits, or interior hyphens")
			}
		}
	}
	return nil
}

func (m *Manager) validateUENetworkEnvironment(plan ueNetworkPlan) error {
	userDB := m.cfg.UserDBPath()
	if err := ensureUserDB(userDB, m.cfg.ConfDir); err != nil {
		return err
	}
	subscriber.Mutex.Lock()
	records, err := subscriber.Load(userDB, m.cfg.MaxUploadBytes)
	subscriber.Mutex.Unlock()
	if err != nil {
		return fmt.Errorf("validate subscriber database: %w", err)
	}
	networkAddr := plan.Subnet.Addr()
	gatewayAddr := networkAddr.Next()
	broadcastBytes := networkAddr.As4()
	broadcastBytes[3] = 255
	broadcastAddr := netip.AddrFrom4(broadcastBytes)
	for _, record := range records {
		if record.IPAlloc == "dynamic" {
			continue
		}
		addr, err := netip.ParseAddr(record.IPAlloc)
		if err != nil || !addr.Is4() || !plan.Subnet.Contains(addr) ||
			addr == networkAddr || addr == gatewayAddr || addr == broadcastAddr {
			return fmt.Errorf("subscriber %q static ip_alloc %q must be a usable host in %s excluding gateway %s",
				record.Name, record.IPAlloc, plan.SubnetText, plan.SGIAddress)
		}
	}
	configured, err := interfaceNetworks()
	if err != nil {
		return fmt.Errorf("inspect configured interface networks: %w", err)
	}
	for _, candidate := range configured {
		if candidate.Name == sgiInterface || !candidate.Prefix.Addr().Is4() {
			continue
		}
		candidate.Prefix = candidate.Prefix.Masked()
		if plan.Subnet.Contains(candidate.Prefix.Addr()) || candidate.Prefix.Contains(plan.Subnet.Addr()) {
			return fmt.Errorf("ue_subnet %s overlaps interface %q network %s", plan.SubnetText, candidate.Name, candidate.Prefix)
		}
	}
	return nil
}

func currentInterfaceNetworks() ([]interfaceNetwork, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []interfaceNetwork
	for _, iface := range ifaces {
		if iface.Name == sgiInterface {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("interface %q addresses: %w", iface.Name, err)
		}
		for _, raw := range addrs {
			prefix, err := netip.ParsePrefix(raw.String())
			if err == nil && prefix.Addr().Is4() {
				out = append(out, interfaceNetwork{Name: iface.Name, Prefix: prefix.Masked()})
			}
		}
	}
	return out, nil
}

func (m *Manager) NetworkPlanSnapshot() NetworkPlanSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	active := alive(m.epcCmd) && alive(m.enbCmd)
	p := m.lastStart
	normalizeUEPolicy(&p)
	plan, err := makeUENetworkPlan(p.UESubnet, p.UEAccess)
	if err != nil {
		plan, _ = makeUENetworkPlan(defaultUESubnet, defaultUEAccess)
	}
	snapshot := NetworkPlanSnapshot{
		Active: active, UESubnet: plan.SubnetText, SGIAddress: plan.SGIAddress, UEAccess: plan.Access,
	}
	if active {
		snapshot.ResolvedNetwork = m.lastNetwork
	}
	return snapshot
}
