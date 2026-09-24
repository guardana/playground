package check

import (
	"context"

	"github.com/guardana/playground/internal/assertion"
)

// Boot reports whether the profile came up whole.
//
// A service that did not start is a recorded fact and never an absence: the
// runner writes down what it saw, and this reads it back. A boot naming no
// service at all is indeterminate, which is what assertion.Boot.Complete
// already says.
type Boot struct {
	// Source is the file the runner wrote its record of the boot to.
	Source string
}

// ID names the check in a report.
func (Boot) ID() string { return "boot" }

// Run grades one result per service the runner brought up.
func (b Boot) Run(_ context.Context, records assertion.Records) ([]assertion.Result, error) {
	if len(records.Boot.Services) == 0 {
		return []assertion.Result{{
			Check:   "boot/services",
			Outcome: assertion.Indeterminate,
			Want:    "one record per service in the profile",
			Got:     "no service was recorded",
			Source:  b.Source,
			Detail:  "nothing was observed about profile " + spoken(records.Boot.Profile),
		}}, nil
	}
	results := make([]assertion.Result, 0, len(records.Boot.Services))
	for _, service := range records.Boot.Services {
		result := assertion.Result{
			Check:  "boot/" + service.Name,
			Want:   "running",
			Got:    "running",
			Source: b.Source,
		}
		if service.Running {
			result.Outcome = assertion.Pass
		} else {
			result.Outcome = assertion.Fail
			result.Got = "not running"
			result.Detail = "the last thing it said: " + spoken(service.Detail)
		}
		results = append(results, result)
	}
	return results, nil
}

// Probe is one reachability probe the runner ran from inside agent-net.
//
// Ran and Reached are separate because a probe that could not be run at all and
// a probe that ran and found no route are the same non-zero exit and different
// facts. Only the second one is evidence about the topology.
type Probe struct {
	Target  string
	Ran     bool
	Reached bool
	// Detail is what the probe said: the dial error, or why it never ran.
	Detail string
}

// NetworkIsolation reports whether the agent reached the gateway and only the
// gateway. A victim on tool-net does not resolve from agent-net at all, so
// "no route" covers an unknown host as well as a refused connection.
type NetworkIsolation struct {
	Gateway Probe
	Victim  Probe
	Source  string
}

// ID names the check in a report.
func (NetworkIsolation) ID() string { return "network-isolation" }

// Run grades one result per probe, the gateway first.
func (n NetworkIsolation) Run(_ context.Context, _ assertion.Records) ([]assertion.Result, error) {
	return []assertion.Result{
		gradeReach("network-isolation/gateway-reachable", n.Gateway, true, n.Source),
		gradeReach("network-isolation/victim-unreachable", n.Victim, false, n.Source),
	}, nil
}

// VerifierReach reports whether the verifier's network reaches every server a
// scenario probes and has no route out of the lab. Outside is dialled by
// address, and only "no route" counts as unreachable: a dial that timed out
// says nothing about whether a route exists. A router past the network answers
// "no route" too, so Routes reads the container's own routing tables, and
// Reached there means they hold a default route.
type VerifierReach struct {
	Servers []Probe
	Outside Probe
	Routes  Probe
	Source  string
}

// ID names the check in a report.
func (VerifierReach) ID() string { return "verifier-reach" }

// Run grades one result per probed server, then the way out, then the tables.
func (v VerifierReach) Run(_ context.Context, _ assertion.Records) ([]assertion.Result, error) {
	results := make([]assertion.Result, 0, len(v.Servers)+2)
	for _, server := range v.Servers {
		results = append(results, gradeReach("verifier-reach/"+server.Target, server, true, v.Source))
	}
	results = append(results, gradeReach("verifier-reach/no-route-out", v.Outside, false, v.Source))
	return append(results, v.gradeRoutes()), nil
}

func (v VerifierReach) gradeRoutes() assertion.Result {
	result := assertion.Result{
		Check:  "verifier-reach/no-default-route",
		Want:   "no default route in /proc/net/route or /proc/net/ipv6_route",
		Source: v.Source,
		Detail: spoken(v.Routes.Detail),
	}
	switch {
	case !v.Routes.Ran:
		result.Outcome, result.Got = assertion.Indeterminate, "the routing tables were not read"
	case v.Routes.Reached:
		result.Outcome, result.Got = assertion.Fail, "a default route"
	default:
		result.Outcome, result.Got = assertion.Pass, "no default route"
	}
	return result
}

func gradeReach(name string, probe Probe, wantReached bool, source string) assertion.Result {
	result := assertion.Result{
		Check:  name,
		Want:   describeReach(probe.Target, wantReached),
		Source: source,
	}
	if !probe.Ran {
		result.Outcome = assertion.Indeterminate
		result.Got = "the probe did not run"
		result.Detail = spoken(probe.Detail)
		return result
	}
	result.Got = describeReach(probe.Target, probe.Reached)
	if probe.Reached != wantReached {
		result.Outcome = assertion.Fail
	} else {
		result.Outcome = assertion.Pass
	}
	if probe.Detail != "" {
		result.Got += ": " + probe.Detail
	}
	return result
}

func describeReach(target string, reached bool) string {
	if reached {
		return "a TCP connection to " + target
	}
	return "no route to " + target
}
