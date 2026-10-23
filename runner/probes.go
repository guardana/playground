package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
	"github.com/guardana/playground/runner/dockercompose"
	"github.com/guardana/playground/runner/driver"
)

// isolation asks, from inside agent-net, whether the gate answers and the
// victim the trajectory calls first does not, and has the gate probe what it
// keeps from the agent. Every answer is recorded, in memory for the check and
// on disk for the person the check sends to a file. None is inferred from
// another: a topology where nothing at all is reachable would otherwise look
// like a topology that is right about the victim.
func (l lab) isolation(
	ctx context.Context, run driver.Run, spec labspec.Scenario, trajectory labspec.Trajectory, gate driver.GateRun,
) check.NetworkIsolation {
	gateway := l.probe(ctx, run.Compose, spec.Profile, gate.Endpoint())
	victim := l.probe(ctx, run.Compose, spec.Profile, trajectory.Steps[0].Call.Server+":"+servicePort)
	sealed := gate.Sealed(ctx, func(ctx context.Context, target string) check.Probe {
		return l.probe(ctx, run.Compose, spec.Profile, target)
	})
	source := l.recordProbes(run.Dir, gateway, victim, sealed)
	return check.NetworkIsolation{Gateway: gateway, Victim: victim, Sealed: sealed, Source: source}
}

// recordProbes writes probes.log, one line for each probe, and returns its
// path.
func (l lab) recordProbes(runDir string, gateway, victim check.Probe, sealed []check.Probe) string {
	source := filepath.Join(runDir, "probes.log")
	recorded := fmt.Sprintf("gateway %s ran=%t reached=%t %s\nvictim %s ran=%t reached=%t %s\n",
		gateway.Target, gateway.Ran, gateway.Reached, gateway.Detail,
		victim.Target, victim.Ran, victim.Reached, victim.Detail)
	for _, probe := range sealed {
		kind := "sealed " + probe.Target
		switch probe.Kind {
		case check.ListenerByName, check.ListenerByAddress:
			kind = "listener " + probe.Target + " from " + probe.From
		case check.ListenerLog:
			kind = "listener-log " + probe.Target
		}
		recorded += fmt.Sprintf("%s ran=%t reached=%t %s\n", kind, probe.Ran, probe.Reached, probe.Detail)
	}
	if err := writeBytes(source, []byte(recorded)); err != nil {
		l.note("writing the probe record: %v", err)
	}
	return source
}

func (l lab) probe(ctx context.Context, compose dockercompose.Compose, profiles []string, target string) check.Probe {
	execution, err := compose.RunOnce(ctx, profiles, agentService, []string{"-probe", target})
	if err != nil {
		return check.Probe{Target: target, Detail: err.Error()}
	}
	return readProbe(target, execution.Output)
}

// The two markers agents/scripted prints for -probe. They are written out here
// rather than imported because the agent is a separate command and the frozen
// contract packages are not the place for a private marker; changing one
// without the other breaks the network-isolation check, so each side names the
// other in a comment.
const (
	probeReached     = "probe reached"
	probeUnreachable = "probe unreachable"
)

// readProbe turns what the agent printed into what the check reads. The marker
// is what separates "the agent ran and found no route" from "the agent could
// not be run", which have the same non-zero exit and are different facts.
func readProbe(target, output string) check.Probe {
	trimmed := strings.TrimSpace(output)
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, probeReached):
			return check.Probe{Target: target, Ran: true, Reached: true, Detail: line}
		case strings.HasPrefix(line, probeUnreachable):
			return check.Probe{Target: target, Ran: true, Detail: line}
		}
	}
	if trimmed == "" {
		trimmed = "the probe printed nothing"
	}
	return check.Probe{Target: target, Detail: trimmed}
}
