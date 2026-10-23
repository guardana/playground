package main

import (
	"encoding/binary"
	"hash/fnv"
	"maps"
	"net/netip"
	"path/filepath"
	"strings"

	"github.com/guardana/playground/runner/driver"
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
// to the agent's containers, and the gate's fixed address in the upper half.
type agentNet struct {
	subnet  netip.Prefix
	dynamic netip.Prefix
	gate    netip.Addr
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
		subnet:  netip.PrefixFrom(start, agentBits),
		dynamic: netip.PrefixFrom(start, agentBits+1),
		gate:    netip.AddrFrom4(last),
	}
}

// newRun is one scenario's run, with the gate's address on its agent-net.
func newRun(runID, runDir string) driver.Run {
	return driver.Run{ID: runID, Dir: runDir, GateAddress: agentNetFor(runID).gate.String()}
}

// environment is what compose interpolates into the topology for this run:
// the run's own variables, and every subject's, whichever the scenario picks.
func (l lab) environment(run driver.Run) map[string]string {
	env := map[string]string{
		"LAB_RUN_ID":           run.ID,
		"LAB_REPORTS_DIR":      containerReports,
		"COMPOSE_PROJECT_NAME": projectName(run.ID),
		workspaceVariable:      l.workspace.dir,
	}
	network := agentNetFor(run.ID)
	env["LAB_AGENT_SUBNET"] = network.subnet.String()
	env["LAB_AGENT_RANGE"] = network.dynamic.String()
	for _, subject := range l.subjects() {
		maps.Copy(env, subject.Env(run))
	}
	if absolute, err := filepath.Abs(run.Dir); err == nil {
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
