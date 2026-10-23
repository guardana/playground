package control

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
	"github.com/guardana/playground/runner/dockercompose"
)

const (
	pdpProfile = "pdp"
	pdpService = "pdp-double"
	// maxProbeDetail bounds what one probe line keeps of a listing.
	maxProbeDetail = 512
)

// listenerProbes asks about the enforcer's agent listener three ways: services
// that share a network with the enforcer other than agent-net (the victim the
// trajectory calls first, the decision point double when it is up) list its
// tools at its name, which there resolves to its address on that network, and
// at its agent-net address; and the enforcer's own output says where it bound.
// Each probe records what it saw; the check decides what that shows.
func listenerProbes(
	ctx context.Context, compose dockercompose.Compose, spec labspec.Scenario, trajectory labspec.Trajectory, agent string,
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

func listing(kind check.ListenerKind, target, service, agent string, listed dockercompose.Split, err error) check.Probe {
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
