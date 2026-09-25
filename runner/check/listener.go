package check

import (
	"net/netip"
	"regexp"
	"strings"

	"github.com/guardana/playground/internal/assertion"
)

// ListenerKind is what a listener probe asked about the enforcer's agent
// listener.
type ListenerKind int

const (
	// ListenerByName is a service on another of the enforcer's networks
	// listing its tools at the enforcer's service name, which there resolves
	// to the enforcer's address on the network the two share.
	ListenerByName ListenerKind = iota + 1
	// ListenerByAddress is the same service listing them at the enforcer's
	// address on agent-net, the one an attacker there would dial.
	ListenerByAddress
	// ListenerLog is what the enforcer printed as it bound its listener.
	ListenerLog
)

var (
	refusedDial = regexp.MustCompile(`dial tcp ([^ ]+): connect: connection refused`)
	boundOn     = regexp.MustCompile(`listening for agents on ([^\s;,]+)`)
)

// gradeListener reads a listener probe. A result passes only on the one
// answer that shows the listener closed; an error the probe did not name
// measured nothing and is indeterminate.
func gradeListener(probe Probe, source string) assertion.Result {
	result := assertion.Result{Source: source, Detail: spoken(probe.Detail)}
	switch probe.Kind {
	case ListenerByName:
		result.Check = "network-isolation/listener-closed-to/" + probe.From
		result.Want = "connection refused at " + probe.Target + " from " + probe.From + ", on its own network"
	case ListenerByAddress:
		result.Check = "network-isolation/agent-address-unreachable-from/" + probe.From
		result.Want = "no route to, or no answer from, " + probe.Target + " from " + probe.From
	default:
		result.Check = "network-isolation/listener-bound-to-agent-net"
		result.Want = "the enforcer says its agent listener is bound to " + probe.Target + " alone"
	}
	switch {
	case !probe.Ran:
		result.Outcome, result.Got = assertion.Indeterminate, "the probe did not run"
	case probe.Kind != ListenerLog && probe.Reached:
		result.Outcome, result.Got = assertion.Fail, probe.From+" listed the enforcer's tools at "+probe.Target
	case probe.Kind == ListenerByName:
		result.Outcome, result.Got = byName(probe)
	case probe.Kind == ListenerByAddress:
		result.Outcome, result.Got = byAddress(probe)
	default:
		result.Outcome, result.Got = bound(probe)
	}
	return result
}

// byName passes on a refusal of the enforcer's port at an address that is not
// its agent-net one: the listener is not on the network the service shares.
func byName(probe Probe) (assertion.Outcome, string) {
	_, port, _ := strings.Cut(probe.Target, ":")
	for _, match := range refusedDial.FindAllStringSubmatch(probe.Detail, -1) {
		at, err := netip.ParseAddrPort(match[1])
		if err == nil && at.Addr().String() != probe.Agent && strings.HasSuffix(match[1], ":"+port) &&
			strings.Contains(probe.Detail, `Post "http://`+probe.Target+`/mcp": `+match[0]) {
			return assertion.Pass, "connection refused at " + match[1]
		}
	}
	return assertion.Indeterminate, "not measured: the listing failed otherwise than by a refused connection at " + probe.Target
}

// byAddress passes when the agent-net address was out of reach: no route to
// it, or no answer before the listing's deadline. A refusal there means
// something answered at that address.
func byAddress(probe Probe) (assertion.Outcome, string) {
	dial := "dial tcp " + probe.Target + ": connect: "
	switch {
	case strings.Contains(probe.Detail, dial+"network is unreachable"):
		return assertion.Pass, "network is unreachable at " + probe.Target
	case strings.Contains(probe.Detail, dial+"no route to host"):
		return assertion.Pass, "no route to host at " + probe.Target
	case strings.Contains(probe.Detail, `Post "http://`+probe.Target+`/mcp": context deadline exceeded`):
		return assertion.Pass, "no answer from " + probe.Target + " before the listing's deadline"
	case strings.Contains(probe.Detail, dial+"connection refused"):
		return assertion.Fail, "something at " + probe.Target + " answered, with a refused connection"
	}
	return assertion.Indeterminate, "not measured: the listing failed otherwise than by no route or no answer at " + probe.Target
}

// bound reads every address the enforcer said its agent listener took.
func bound(probe Probe) (assertion.Outcome, string) {
	var said []string
	for _, match := range boundOn.FindAllStringSubmatch(probe.Detail, -1) {
		if _, err := netip.ParseAddrPort(match[1]); err == nil {
			said = append(said, match[1])
		}
	}
	if len(said) == 0 {
		return assertion.Indeterminate, "not measured: the enforcer's output names no address its agent listener bound"
	}
	for _, address := range said {
		if address != probe.Target {
			return assertion.Fail, "the enforcer's agent listener is bound to " + address
		}
	}
	return assertion.Pass, "bound to " + probe.Target + " alone"
}
