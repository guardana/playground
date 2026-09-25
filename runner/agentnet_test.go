package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func agentNetOf(t *testing.T, runID string) (subnet, dynamic netip.Prefix, enforcer netip.Addr) {
	t.Helper()
	env := lab{}.environment(runID, t.TempDir())
	var err error
	if subnet, err = netip.ParsePrefix(env["LAB_AGENT_SUBNET"]); err != nil {
		t.Fatalf("LAB_AGENT_SUBNET: %v", err)
	}
	if dynamic, err = netip.ParsePrefix(env["LAB_AGENT_RANGE"]); err != nil {
		t.Fatalf("LAB_AGENT_RANGE: %v", err)
	}
	if enforcer, err = netip.ParseAddr(env["LAB_ENFORCER_ADDRESS"]); err != nil {
		t.Fatalf("LAB_ENFORCER_ADDRESS: %v", err)
	}
	return subnet, dynamic, enforcer
}

// The enforcer's address is its own: inside the run's subnet, outside the part
// docker hands out to the agent's containers, and never the network's own
// address, its gateway or its broadcast.
func TestEachRunGetsAnAgentNetWithTheEnforcerAtAFixedAddress(t *testing.T) {
	pool := netip.MustParsePrefix("10.231.0.0/16")
	byHand := netip.MustParsePrefix("10.231.255.224/27")
	for _, suffix := range []string{"00000000", "4f73420a", "ffffffff", "7fd46c30", "not-hex"} {
		subnet, dynamic, enforcer := agentNetOf(t, "tool-02-20260924T192517Z-"+suffix)
		switch {
		case subnet.Bits() != 27 || subnet != subnet.Masked() || !pool.Contains(subnet.Addr()):
			t.Errorf("%s: subnet %s is not a /27 inside %s", suffix, subnet, pool)
		case subnet == byHand:
			t.Errorf("%s: a run took %s, the subnet a lab brought up by hand uses", suffix, subnet)
		case dynamic.Addr() != subnet.Addr() || dynamic.Bits() != 28:
			t.Errorf("%s: docker hands out %s, want the lower half of %s", suffix, dynamic, subnet)
		case !subnet.Contains(enforcer) || dynamic.Contains(enforcer):
			t.Errorf("%s: the enforcer at %s is outside %s or inside %s", suffix, enforcer, subnet, dynamic)
		case enforcer.Next() == netip.Addr{} || !subnet.Contains(enforcer.Next()):
			t.Errorf("%s: the enforcer at %s is the broadcast address of %s", suffix, enforcer, subnet)
		}
	}
}

// The last /27 of the pool is the one compose defaults to by hand. A suffix
// whose hash lands on it under a plain modulo still gets another subnet.
func TestNoRunTakesTheSubnetOfALabBroughtUpByHand(t *testing.T) {
	suffix := ""
	for i := 0; suffix == "" && i < 1<<20; i++ {
		candidate := fmt.Sprintf("%08x", i)
		hash := fnv.New32a()
		_, _ = hash.Write([]byte(candidate))
		if hash.Sum32()%2048 == 2047 {
			suffix = candidate
		}
	}
	if suffix == "" {
		t.Fatal("no suffix lands on the last slot")
	}
	subnet, _, _ := agentNetOf(t, "tool-02-20260924T192517Z-"+suffix)
	if subnet == netip.MustParsePrefix("10.231.255.224/27") {
		t.Errorf("run suffix %s took %s, the subnet of a lab brought up by hand", suffix, subnet)
	}
}

// Two runs apart only by their suffix, as two parallel runs of one scenario are,
// get two subnets; the same run id always gets the same one.
func TestTheSubnetFollowsTheRunsRandomSuffix(t *testing.T) {
	first, _, _ := agentNetOf(t, "tool-02-20260924T192517Z-4f73420a")
	again, _, _ := agentNetOf(t, "tool-02-20260924T192517Z-4f73420a")
	other, _, _ := agentNetOf(t, "tool-02-20260924T192517Z-7fd46c30")
	if first != again {
		t.Errorf("one run id gave %s and %s", first, again)
	}
	if first == other {
		t.Errorf("two suffixes share %s", first)
	}
	seen := map[netip.Prefix]bool{}
	for i := range 64 {
		subnet, _, _ := agentNetOf(t, "x-20260924T192517Z-"+strings.Repeat("0", 6)+string(rune('a'+i%26))+string(rune('a'+i/26)))
		seen[subnet] = true
	}
	if len(seen) < 60 {
		t.Errorf("64 suffixes landed on %d subnets", len(seen))
	}
}

// The listener the runner writes is the address compose gives the enforcer on
// agent-net in the same run.
func TestTheGatewayListensOnTheAddressComposeGivesTheEnforcer(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	if _, err := subject.execute(context.Background(), scenario); err != nil {
		t.Fatalf("execute: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(compose.env["LAB_RUN_HOST_DIR"], "gateway", "gateway.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var assembled struct {
		Listener struct {
			Address string `json:"address"`
		} `json:"listener"`
	}
	if err := yaml.Unmarshal(body, &assembled); err != nil {
		t.Fatal(err)
	}
	want := compose.env["LAB_ENFORCER_ADDRESS"] + ":" + servicePort
	if compose.env["LAB_ENFORCER_ADDRESS"] == "" || assembled.Listener.Address != want {
		t.Errorf("listener.address is %q, want %q", assembled.Listener.Address, want)
	}
}

// compose reads the three variables the runner sets, where they decide the
// topology: agent-net's subnet and range, and the enforcer's address on it.
func TestComposeTakesTheAgentNetFromTheRunner(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", composeFile))
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Services map[string]struct {
			Networks any `json:"networks"`
		} `json:"services"`
		Networks map[string]struct {
			IPAM struct {
				Config []map[string]string `json:"config"`
			} `json:"ipam"`
		} `json:"networks"`
	}
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	config := parsed.Networks["agent-net"].IPAM.Config
	enforcer, _ := parsed.Services[enforcerService].Networks.(map[string]any)
	onAgentNet, _ := enforcer["agent-net"].(map[string]any)
	for name, value := range map[string]string{
		"LAB_AGENT_SUBNET":     ipamValue(config, "subnet"),
		"LAB_AGENT_RANGE":      ipamValue(config, "ip_range"),
		"LAB_ENFORCER_ADDRESS": stringOf(onAgentNet["ipv4_address"]),
	} {
		if !strings.HasPrefix(value, "${"+name+":-") {
			t.Errorf("compose reads %q where the runner sets %s", value, name)
		}
	}
}

func ipamValue(config []map[string]string, key string) string {
	if len(config) != 1 {
		return ""
	}
	return config[0][key]
}

func stringOf(value any) string {
	found, _ := value.(string)
	return found
}
