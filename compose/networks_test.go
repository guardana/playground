// The topology's own claim, read back from the file that makes it: nothing in
// the lab has a route out. It is asserted here rather than trusted because the
// difference between a sealed network and an open one is one word in a mapping
// nobody reads twice.
package compose

import (
	"encoding/json"
	"errors"
	"maps"
	"net/netip"
	"os"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

type topology struct {
	Networks map[string]struct {
		Internal bool `json:"internal"`
		IPAM     struct {
			Config []map[string]string `json:"config"`
		} `json:"ipam"`
	} `json:"networks"`
	Services map[string]struct {
		Networks    networkList `json:"networks"`
		NetworkMode string      `json:"network_mode"`
	} `json:"services"`
}

// networkList reads a service's networks in either spelling compose accepts,
// a list of names or a mapping from name to settings, as the sorted names.
type networkList []string

func (n *networkList) UnmarshalJSON(body []byte) error {
	var names []string
	if err := json.Unmarshal(body, &names); err == nil {
		*n = names
		return nil
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(body, &settings); err != nil {
		return err
	}
	*n = slices.Sorted(maps.Keys(settings))
	return nil
}

// Services allowed to reach evidence-net: the collector always, and the
// enforcer once a service runs that image. Anything else there could post log
// records of its own and forge evidence.
var evidenceNetServices = map[string]bool{"collector": true, "enforcer": true}

func read(t *testing.T) topology {
	t.Helper()
	body, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var parsed topology
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("compose.yaml: %v", err)
	}
	return parsed
}

// The attack payloads exist to be blocked inside a network with no route out,
// and the agent is the thing they are aimed at: it replays them. A network the
// agent can reach the internet from is the one exfiltration a scenario could
// not detect, because it would succeed. The count is not asserted: lanes add
// networks as they add services that need their own boundary.
func TestEveryLabNetworkIsInternal(t *testing.T) {
	networks := read(t).Networks
	if len(networks) == 0 {
		t.Fatal("the lab declares no networks")
	}
	for name, network := range networks {
		if !network.Internal {
			t.Errorf("%s is not internal, so everything on it has a route out of the lab", name)
		}
	}
}

// network_mode replaces compose's own network isolation with the host's or
// another container's, which no hardened lab service may do: it would leave
// that service on whatever network the mode names, unchecked by this file.
func TestNoServiceSetsNetworkMode(t *testing.T) {
	for name, service := range read(t).Services {
		if service.NetworkMode != "" {
			t.Errorf("%s sets network_mode: %s, which bypasses every network check here", name, service.NetworkMode)
		}
	}
}

// A victim that could reach the collector could post log records of its own
// and forge evidence. Only the collector and the enforcer (once it runs) may
// be on evidence-net.
func TestEvidenceNetHoldsOnlyTheCollectorAndTheEnforcer(t *testing.T) {
	services := read(t).Services
	collector, declared := services["collector"]
	if !declared {
		t.Fatal("compose.yaml declares no collector")
	}
	if len(collector.Networks) != 1 || collector.Networks[0] != "evidence-net" {
		t.Errorf("collector is on %v, want evidence-net alone", collector.Networks)
	}
	for name, service := range services {
		if evidenceNetServices[name] {
			continue
		}
		if slices.Contains(service.Networks, "evidence-net") {
			t.Errorf("%s is on evidence-net, which only the collector and the enforcer may reach", name)
		}
	}
}

// The agent reaches the gateway and nothing else. A victim on agent-net would
// be a call that never crossed the thing under test.
func TestTheAgentIsOnAgentNetAlone(t *testing.T) {
	agent, declared := read(t).Services["scripted-agent"]
	if !declared {
		t.Fatal("compose.yaml declares no scripted-agent")
	}
	if len(agent.Networks) != 1 || agent.Networks[0] != "agent-net" {
		t.Errorf("scripted-agent is on %v, want agent-net alone", agent.Networks)
	}
}

// The enforcer's agent listener binds its address on agent-net alone, so that
// address has to be its own: fixed, and outside the range docker hands out to
// the agent's containers. The defaults are what a lab brought up by hand gets.
func TestTheEnforcerHasAFixedAddressOnAgentNet(t *testing.T) {
	parsed := read(t)
	enforcer := parsed.Services["enforcer"].Networks
	if !slices.Equal(enforcer, []string{"agent-net", "evidence-net", "pdp-net", "tool-net"}) {
		t.Errorf("the enforcer is on %v", enforcer)
	}
	var fixed struct {
		Services struct {
			Enforcer struct {
				Networks map[string]struct {
					IPv4Address string `json:"ipv4_address"`
				} `json:"networks"`
			} `json:"enforcer"`
		} `json:"services"`
	}
	body, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(body, &fixed); err != nil {
		t.Fatal(err)
	}
	config := parsed.Networks["agent-net"].IPAM.Config
	if len(config) != 1 {
		t.Fatalf("agent-net declares %d address pools, want one", len(config))
	}
	subnet, errSubnet := netip.ParsePrefix(byHand(config[0]["subnet"]))
	dynamic, errRange := netip.ParsePrefix(byHand(config[0]["ip_range"]))
	address, errAddress := netip.ParseAddr(byHand(fixed.Services.Enforcer.Networks["agent-net"].IPv4Address))
	if err := errors.Join(errSubnet, errRange, errAddress); err != nil {
		t.Fatalf("agent-net's defaults do not read: %v", err)
	}
	if !subnet.Contains(address) || dynamic.Contains(address) || !subnet.Contains(dynamic.Addr()) {
		t.Errorf("the enforcer at %s is not in %s outside the range %s docker hands out", address, subnet, dynamic)
	}
}

// byHand is the default of a `${NAME:-default}` interpolation.
func byHand(value string) string {
	_, fallback, found := strings.Cut(value, ":-")
	if !found || !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
		return ""
	}
	return strings.TrimSuffix(fallback, "}")
}
