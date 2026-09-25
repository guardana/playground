package main

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"net"
	"net/netip"
	"path/filepath"
	"strings"
)

// agentPool is where every run's agent-net is carved from: private, and outside
// the pools docker allocates its own networks from. The last /27 is left to a
// lab brought up by hand, which compose.yaml defaults to.
const (
	agentPool  = "10.231.0.0/16"
	agentBits  = 27
	agentSlots = 1<<(agentBits-16) - 1
)

// agentNet is one run's agent-net: its subnet, the lower half docker hands out
// to the agent's containers, and the enforcer's fixed address in the upper half.
type agentNet struct {
	subnet   netip.Prefix
	dynamic  netip.Prefix
	enforcer netip.Addr
}

// agentNetFor picks the run's subnet from its random suffix, so parallel runs
// land apart. Two runs that land on one subnet are not resolved here: docker
// refuses the second network, and that run's boot fails.
func agentNetFor(runID string) agentNet {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(runID[strings.LastIndex(runID, "-")+1:]))
	slot := hash.Sum32() % agentSlots
	base := netip.MustParsePrefix(agentPool).Addr().As4()
	offset := binary.BigEndian.Uint32(base[:]) + slot<<(32-agentBits)
	var first [4]byte
	binary.BigEndian.PutUint32(first[:], offset)
	var last [4]byte
	binary.BigEndian.PutUint32(last[:], offset+1<<(32-agentBits)-2)
	start := netip.AddrFrom4(first)
	return agentNet{
		subnet:   netip.PrefixFrom(start, agentBits),
		dynamic:  netip.PrefixFrom(start, agentBits+1),
		enforcer: netip.AddrFrom4(last),
	}
}

// enforcerListener is the address the enforcer's agent listener binds in this
// run: its own address on agent-net, which no other network of its reaches.
func enforcerListener(runID string) string {
	return net.JoinHostPort(agentNetFor(runID).enforcer.String(), servicePort)
}

// environment is what compose interpolates into the topology for this run.
func (l lab) environment(runID, runDir string) map[string]string {
	env := map[string]string{
		"LAB_RUN_ID":           runID,
		"LAB_REPORTS_DIR":      containerReports,
		"COMPOSE_PROJECT_NAME": projectName(runID),
		workspaceVariable:      l.workspace.dir,
	}
	network := agentNetFor(runID)
	env["LAB_AGENT_SUBNET"] = network.subnet.String()
	env["LAB_AGENT_RANGE"] = network.dynamic.String()
	env["LAB_ENFORCER_ADDRESS"] = network.enforcer.String()
	if absolute, err := filepath.Abs(runDir); err == nil {
		env["LAB_RUN_HOST_DIR"] = absolute
	}
	return env
}

// projectName gives every run its own compose project, so two runs share no
// container, network or volume and a run that crashed leaves nothing the next
// one boots into. The random suffix of the run id is what makes it unique.
func projectName(runID string) string {
	return "lab-" + strings.ToLower(runID[strings.LastIndex(runID, "-")+1:])
}

// enforcerNamespace is the namespace versions.env pins for the enforcer. Its
// gateway marks the answers it makes itself under it, and the agent is told it
// so an upstream result shaped like a pending answer is not retried as one.
func enforcerNamespace(root string) (string, error) {
	pins, err := readPins(filepath.Join(root, versionFile))
	if err != nil {
		return "", fmt.Errorf("the pins cannot be read: %w", err)
	}
	namespace := pinValue(pins, "ENFORCER_NAMESPACE")
	if namespace == "" {
		return "", fmt.Errorf("%s pins no ENFORCER_NAMESPACE", versionFile)
	}
	return namespace, nil
}

// enforcerPin is the commit versions.env pins the enforcer at.
func enforcerPin(root string) (string, error) {
	pins, err := readPins(filepath.Join(root, versionFile))
	if err != nil {
		return "", fmt.Errorf("the pins cannot be read: %w", err)
	}
	pin := pinValue(pins, "ENFORCER_COMMIT")
	if pin == "" {
		return "", fmt.Errorf("%s pins no ENFORCER_COMMIT", versionFile)
	}
	return pin, nil
}

// enforcerImage is the enforcer's image as versions.env pins it, name and tag.
func enforcerImage(root string) (string, error) {
	pins, err := readPins(filepath.Join(root, versionFile))
	if err != nil {
		return "", fmt.Errorf("the pins cannot be read: %w", err)
	}
	name, commit := pinValue(pins, "ENFORCER_IMAGE"), pinValue(pins, "ENFORCER_COMMIT")
	if name == "" || commit == "" {
		return "", fmt.Errorf("%s pins no ENFORCER_IMAGE or no ENFORCER_COMMIT", versionFile)
	}
	return name + ":" + commit, nil
}

// lookupIn reads one variable from an environment given as NAME=VALUE lines.
func lookupIn(environ []string) func(string) string {
	return func(name string) string {
		for _, entry := range environ {
			if key, value, ok := strings.Cut(entry, "="); ok && key == name {
				return value
			}
		}
		return ""
	}
}
