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
		n.grade("network-isolation/gateway-reachable", n.Gateway, true),
		n.grade("network-isolation/victim-unreachable", n.Victim, false),
	}, nil
}

func (n NetworkIsolation) grade(name string, probe Probe, wantReached bool) assertion.Result {
	result := assertion.Result{
		Check:  name,
		Want:   describeReach(probe.Target, wantReached),
		Source: n.Source,
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
