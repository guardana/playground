// The topology's own claim, read back from the file that makes it: nothing in
// the lab has a route out. It is asserted here rather than trusted because the
// difference between a sealed network and an open one is one word in a mapping
// nobody reads twice.
package compose

import (
	"os"
	"testing"

	"sigs.k8s.io/yaml"
)

type topology struct {
	Networks map[string]struct {
		Internal bool `json:"internal"`
	} `json:"networks"`
	Services map[string]struct {
		Networks []string `json:"networks"`
	} `json:"services"`
}

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
// not detect, because it would succeed.
func TestEveryLabNetworkIsInternal(t *testing.T) {
	networks := read(t).Networks
	if len(networks) != 2 {
		t.Fatalf("the lab declares %d networks, want agent-net and tool-net", len(networks))
	}
	for name, network := range networks {
		if !network.Internal {
			t.Errorf("%s is not internal, so everything on it has a route out of the lab", name)
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
