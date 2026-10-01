package main

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

const (
	pdpProfile = "pdp"
	pdpService = "pdp-double"
	// maxProbeDetail bounds what one probe line keeps of a listing.
	maxProbeDetail = 512
)

// probes asks, from inside agent-net, whether the gateway answers and one
// victim does not. Both answers are recorded, in memory for the check and on
// disk for the person the check sends to a file. Neither is inferred from the
// other: a topology where nothing at all is reachable would otherwise look like
// a topology that is right about the victim. The listener probes, which run the
// other way, travel among the sealed ones.
func (l lab) probes(
	ctx context.Context, compose Compose, spec labspec.Scenario, trajectory labspec.Trajectory, runDir string,
) (check.Probe, check.Probe, []check.Probe, string) {
	gateway := l.probe(ctx, compose, spec.Profile, enforcerService+":"+servicePort)
	victim := l.probe(ctx, compose, spec.Profile, trajectory.Steps[0].Call.Server+":"+servicePort)
	proxies, unnamed := l.sealedProxies(spec)
	var sealed []check.Probe
	for _, target := range append(sealedFromAgent(spec), proxies...) {
		sealed = append(sealed, l.probe(ctx, compose, spec.Profile, target))
	}
	if unnamed != nil {
		sealed = append(sealed, check.Probe{Target: proxyService, Detail: unnamed.Error()})
	}
	sealed = append(sealed, listenerProbes(ctx, compose, spec, trajectory, agentNetFor(filepath.Base(runDir)).enforcer.String())...)

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
	return gateway, victim, sealed, source
}

func (l lab) probe(ctx context.Context, compose Compose, profiles []string, target string) check.Probe {
	execution, err := compose.RunOnce(ctx, profiles, agentService, []string{"-probe", target})
	if err != nil {
		return check.Probe{Target: target, Detail: err.Error()}
	}
	return readProbe(target, execution.Output)
}

// listenerProbes asks about the enforcer's agent listener three ways: services
// that share a network with the enforcer other than agent-net (the victim the
// trajectory calls first, the decision point double when it is up) list its
// tools at its name, which there resolves to its address on that network, and
// at its agent-net address; and the enforcer's own output says where it bound.
// Each probe records what it saw; the check decides what that shows.
func listenerProbes(
	ctx context.Context, compose Compose, spec labspec.Scenario, trajectory labspec.Trajectory, agent string,
) []check.Probe {
	from := []string{trajectory.Steps[0].Call.Server}
	if slices.Contains(spec.Profile, pdpProfile) {
		from = append(from, pdpService)
	}
	atAgent := net.JoinHostPort(agent, servicePort)
	probes := make([]check.Probe, 0, 2*len(from)+1)
	for _, service := range from {
		for _, asked := range []struct {
			kind   check.ListenerKind
			target string
		}{{check.ListenerByName, enforcerService + ":" + servicePort}, {check.ListenerByAddress, atAgent}} {
			listed, err := compose.Exec(ctx, spec.Profile, service, []string{"/relist", "http://" + asked.target + "/mcp"})
			probes = append(probes, listing(asked.kind, asked.target, service, agent, listed, err))
		}
	}
	said, err := compose.Logs(ctx, spec.Profile, enforcerService)
	probe := check.Probe{Kind: check.ListenerLog, Target: atAgent, From: enforcerService, Agent: agent, Ran: err == nil}
	if err != nil {
		probe.Detail = oneLine(err.Error())
	} else {
		probe.Detail = oneLine(boundLines(said.Stdout + "\n" + said.Stderr))
	}
	return append(probes, probe)
}

func listing(kind check.ListenerKind, target, service, agent string, listed Split, err error) check.Probe {
	probe := check.Probe{Kind: kind, Target: target, From: service, Agent: agent, Ran: err == nil}
	switch {
	case err != nil:
		probe.Detail = err.Error()
	case listed.ExitCode == 0:
		probe.Reached, probe.Detail = true, listed.Stdout
	default:
		probe.Detail = fmt.Sprintf("exit %d: %s", listed.ExitCode, listed.Stderr)
	}
	probe.Detail = oneLine(probe.Detail)
	return probe
}

// boundLines keeps the lines where the enforcer says what its agent listener
// bound; the rest of its output is not this probe's.
func boundLines(output string) string {
	var kept []string
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "listening for agents on ") {
			kept = append(kept, strings.TrimSpace(line))
		}
	}
	return strings.Join(kept, "; ")
}

// oneLine keeps a probe's detail to one bounded line of probes.log.
func oneLine(detail string) string {
	joined := strings.Join(strings.Fields(detail), " ")
	if len(joined) > maxProbeDetail {
		return joined[:maxProbeDetail] + "..."
	}
	return joined
}
