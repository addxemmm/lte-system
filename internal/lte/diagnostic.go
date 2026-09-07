package lte

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"slices"
)

// NetworkDiagnostics is configuration evidence collected entirely inside the
// process's current network namespace. It deliberately does not send packets,
// query DNS, mutate sysctls, or repair firewall rules.
type NetworkDiagnostics struct {
	State            string                 `json:"state"`
	Evidence         string                 `json:"evidence"`
	RequestedNetwork string                 `json:"requested_network,omitempty"`
	ResolvedNetwork  string                 `json:"resolved_network,omitempty"`
	UESubnet         string                 `json:"ue_subnet"`
	SGIAddress       string                 `json:"sgi_address"`
	UEAccess         string                 `json:"ue_access"`
	DefaultRoute     DefaultRouteDiagnostic `json:"default_route"`
	SGI              InterfaceDiagnostic    `json:"sgi"`
	IPv4Forward      IPv4ForwardDiagnostic  `json:"ipv4_forward"`
	Rules            NetworkRuleDiagnostics `json:"rules"`
	Problems         []string               `json:"problems,omitempty"`
	Limitations      []string               `json:"limitations,omitempty"`
}

type DefaultRouteDiagnostic struct {
	Present   bool   `json:"present"`
	Interface string `json:"interface,omitempty"`
	Gateway   string `json:"gateway,omitempty"`
	Metric    int    `json:"metric,omitempty"`
}

type InterfaceDiagnostic struct {
	Name    string `json:"name"`
	Present bool   `json:"present"`
}

type IPv4ForwardDiagnostic struct {
	Known   bool   `json:"known"`
	Enabled bool   `json:"enabled"`
	Value   string `json:"value,omitempty"`
}

type NetworkRuleDiagnostics struct {
	NAT    []NetworkRuleDiagnostic `json:"nat"`
	Filter []NetworkRuleDiagnostic `json:"filter"`
	Mangle []NetworkRuleDiagnostic `json:"mangle"`
}

type NetworkRuleDiagnostic struct {
	Name    string `json:"name"`
	Table   string `json:"table"`
	Chain   string `json:"chain"`
	Present bool   `json:"present"`
	Owned   bool   `json:"owned"`
}

// NetworkDiagnostics reports route, forwarding and firewall evidence. Errors
// are data because this endpoint is diagnostic: one unavailable evidence
// source must not discard the other independently collected facts.
func (m *Manager) NetworkDiagnostics(ctx context.Context) NetworkDiagnostics {
	d := NetworkDiagnostics{
		State:    "configured",
		Evidence: "configuration_only",
		SGI:      InterfaceDiagnostic{Name: sgiInterface},
		Limitations: []string{
			"No external host, DNS server, or UE payload is probed.",
			"A route or firewall rule being present does not prove Internet reachability.",
			"Automatic uplink selection reads the current namespace's procfs main IPv4 route table only.",
		},
	}

	m.mu.Lock()
	lastStart := m.lastStart
	d.RequestedNetwork = lastStart.Network
	activeConfig := m.lastNetwork != ""
	activeResolved := m.lastNetwork
	activeSubnet, activeAccess := m.lastUESubnet, m.lastUEAccess
	natOwned := m.natOwned
	forwardOwned := append([]iptRule(nil), m.forwardingOwned...)
	m.mu.Unlock()
	normalizeUEPolicy(&lastStart)
	uePlan, err := makeUENetworkPlan(lastStart.UESubnet, lastStart.UEAccess)
	if err != nil {
		uePlan, _ = makeUENetworkPlan(defaultUESubnet, defaultUEAccess)
	}
	if activeConfig && activeSubnet != "" && activeAccess != "" {
		if activePlan, activeErr := makeUENetworkPlan(activeSubnet, activeAccess); activeErr == nil {
			uePlan = activePlan
		}
	}
	d.UESubnet, d.SGIAddress, d.UEAccess = uePlan.SubnetText, uePlan.SGIAddress, uePlan.Access

	route, routeErr := readDefaultIPv4Route()
	if routeErr != nil {
		d.Problems = append(d.Problems, routeErr.Error())
	} else {
		d.DefaultRoute = DefaultRouteDiagnostic{
			Present: true, Interface: route.Interface, Gateway: route.Gateway, Metric: route.Metric,
		}
	}

	requested := d.RequestedNetwork
	if requested == "" {
		requested = "auto"
	}
	d.RequestedNetwork = requested
	if activeResolved != "" {
		d.ResolvedNetwork = activeResolved
	} else {
		resolved, err := resolveNetwork(requested)
		if err != nil {
			d.Problems = append(d.Problems, fmt.Sprintf("resolve uplink network %q: %v", requested, err))
		} else {
			d.ResolvedNetwork = resolved
		}
	}

	present, err := interfacePresent(sgiInterface)
	if err != nil {
		d.Problems = append(d.Problems, fmt.Sprintf("inspect SGi interface: %v", err))
	} else {
		d.SGI.Present = present
		if activeConfig && !present {
			d.Problems = append(d.Problems, "SGi interface is absent while LTE network configuration is active")
		}
	}

	value, known, enabled, err := readIPv4Forwarding()
	d.IPv4Forward = IPv4ForwardDiagnostic{Known: known, Enabled: enabled, Value: value}
	if err != nil {
		d.Problems = append(d.Problems, fmt.Sprintf("read net.ipv4.ip_forward: %v", err))
	} else if known && !enabled {
		d.Problems = append(d.Problems, fmt.Sprintf("net.ipv4.ip_forward=%q, want 1", value))
	}

	if d.ResolvedNetwork != "" {
		nat := natRule(d.ResolvedNetwork, uePlan.SubnetText)
		d.Rules.NAT = append(d.Rules.NAT, diagnoseRule(ctx, "ue_masquerade", nat, natOwned, activeConfig, &d.Problems))
		for i, rule := range forwardRules(d.ResolvedNetwork, uePlan) {
			name := []string{"ue_to_uplink", "established_to_ue", "mss_to_uplink", "mss_to_ue", "ue_isolation"}[i]
			if uePlan.Access == "allow" && i == 4 {
				name = "ue_interconnect"
			}
			owned := slices.ContainsFunc(forwardOwned, func(candidate iptRule) bool {
				return sameRule(candidate, rule)
			})
			rd := diagnoseRule(ctx, name, rule, owned, activeConfig, &d.Problems)
			if rule.table == "mangle" {
				d.Rules.Mangle = append(d.Rules.Mangle, rd)
			} else {
				d.Rules.Filter = append(d.Rules.Filter, rd)
			}
		}
	}

	if d.ResolvedNetwork == "" || !d.DefaultRoute.Present || (!d.IPv4Forward.Known && runtime.GOOS == "linux") {
		d.State = "unavailable"
	} else if len(d.Problems) != 0 {
		d.State = "degraded"
	}
	return d
}

func diagnoseRule(ctx context.Context, name string, rule iptRule, owned, required bool, problems *[]string) NetworkRuleDiagnostic {
	table := rule.table
	if table == "" {
		table = "filter"
	}
	rd := NetworkRuleDiagnostic{Name: name, Table: table, Chain: rule.chain, Owned: owned}
	out, err := runNetworkCommand(ctx, "iptables", rule.commandArgs("-C")...)
	if err == nil {
		rd.Present = true
		return rd
	}
	if ctx.Err() != nil {
		*problems = append(*problems, fmt.Sprintf("check %s rule: %v", name, ctx.Err()))
		return rd
	}
	ordinaryAbsence := isMissingRule(out, err)
	if !ordinaryAbsence {
		*problems = append(*problems, fmt.Sprintf("check %s rule: %v", name, err))
	} else if required {
		*problems = append(*problems, fmt.Sprintf("required %s rule is absent", name))
	}
	return rd
}

func sameRule(a, b iptRule) bool {
	return a.table == b.table && a.chain == b.chain && slices.Equal(a.args, b.args)
}

func interfacePresent(name string) (bool, error) {
	entries, err := os.ReadDir(netInterfaceDir)
	if err != nil {
		if runtime.GOOS != "linux" && os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	for _, entry := range entries {
		if entry.Name() == name {
			return true, nil
		}
	}
	return false, nil
}
