package check_test

import (
	"context"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/runner/check"
)

const (
	agentAddress = "10.231.4.62"
	atAgent      = agentAddress + ":8080"
	refusedDial  = `calling "initialize": Post "http://enforcer:8080/mcp": dial tcp 172.30.0.4:8080: connect: connection refused`
	agentURL     = `Post "http://` + atAgent + `/mcp": `
)

type listenerCase struct {
	probe check.Probe
	check string
	want  assertion.Outcome
	got   string
}

func byName(detail string) check.Probe {
	return check.Probe{Kind: check.ListenerByName, Target: "enforcer:8080", From: "victim-fs", Agent: agentAddress, Ran: true, Detail: detail}
}

func byAddress(detail string) check.Probe {
	return check.Probe{Kind: check.ListenerByAddress, Target: atAgent, From: "victim-fs", Agent: agentAddress, Ran: true, Detail: detail}
}

func enforcerSaid(detail string) check.Probe {
	return check.Probe{Kind: check.ListenerLog, Target: atAgent, From: "enforcer", Agent: agentAddress, Ran: true, Detail: detail}
}

// At its service name the enforcer is the address it has on the network the
// two share, and only a connection refused there shows the listener is not on
// it. A refusal anywhere else, a docker daemon's included, measured nothing.
func TestTheListenerIsClosedOnlyWhenTheEnforcersOtherAddressRefused(t *testing.T) {
	const name = "network-isolation/listener-closed-to/victim-fs"
	runListenerCases(t, map[string]listenerCase{
		"refused at the shared network's address": {byName(refusedDial), name, assertion.Pass, "connection refused"},
		"listed the enforcer's tools": {byName(`{"name":"fs.read","description":"Read a file."}`) /* reached below */, name,
			assertion.Fail, "listed"},
		"the name did not resolve": {byName("dial tcp: lookup enforcer on 127.0.0.11:53: no such host"), name,
			assertion.Indeterminate, "not measured"},
		"the command never started": {byName(`exec: "/relist": stat /relist: no such file or directory`), name,
			assertion.Indeterminate, "not measured"},
		"a docker daemon over TCP refused": {byName("Cannot connect to the Docker daemon at tcp://127.0.0.1:8080: " +
			"dial tcp 127.0.0.1:8080: connect: connection refused"), name, assertion.Indeterminate, "not measured"},
		"refused on another port": {byName("dial tcp 172.30.0.4:2375: connect: connection refused"), name,
			assertion.Indeterminate, "not measured"},
		"refused at the agent-net address": {byName(`calling "initialize": Post "http://enforcer:8080/mcp": dial tcp ` +
			atAgent + ": connect: connection refused"), name, assertion.Indeterminate, "not measured"},
		"compose could not run it": {check.Probe{Kind: check.ListenerByName, Target: "enforcer:8080", From: "victim-fs",
			Agent: agentAddress, Detail: "docker: not found"}, name, assertion.Indeterminate, "did not run"},
	})
}

// The enforcer's address on agent-net is what someone on another network would
// dial once they knew it. Only a network that has no route to it, or a listing
// that got no answer at all, shows it is out of their reach.
func TestTheAgentNetAddressIsOutOfReachOnlyWhenNothingAnswered(t *testing.T) {
	const name = "network-isolation/agent-address-unreachable-from/victim-fs"
	runListenerCases(t, map[string]listenerCase{
		"no route on an internal network": {byAddress(agentURL + "dial tcp " + atAgent + ": connect: network is unreachable"),
			name, assertion.Pass, "network is unreachable"},
		"no route to the host": {byAddress(agentURL + "dial tcp " + atAgent + ": connect: no route to host"),
			name, assertion.Pass, "no route to host"},
		"no answer within the listing's deadline": {byAddress("context deadline exceeded sending \"notifications/cancelled\": " +
			"rejected by transport: " + agentURL + "context deadline exceeded"), name, assertion.Pass, "no answer"},
		"listed the enforcer's tools": {byAddress(`{"name":"fs.read","description":"Read a file."}`), name,
			assertion.Fail, "listed"},
		"something at the address refused": {byAddress("dial tcp " + atAgent + ": connect: connection refused"), name,
			assertion.Fail, "answered"},
		"another address was unreachable": {byAddress("dial tcp 10.231.9.9:8080: connect: network is unreachable"), name,
			assertion.Indeterminate, "not measured"},
		"a deadline that names no dial": {byAddress("context deadline exceeded"), name,
			assertion.Indeterminate, "not measured"},
		"the command never started": {byAddress(`exec: "/relist": stat /relist: no such file or directory`), name,
			assertion.Indeterminate, "not measured"},
	})
}

// The enforcer prints where its agent listener bound as it binds it. Its own
// word covers every network it is on, the ones no probe runs from included.
func TestTheEnforcerSaysItListensOnItsAgentNetAddressAlone(t *testing.T) {
	const name = "network-isolation/listener-bound-to-agent-net"
	said := "listening for agents on "
	runListenerCases(t, map[string]listenerCase{
		"its agent-net address": {enforcerSaid(said + atAgent), name, assertion.Pass, atAgent},
		"every interface":       {enforcerSaid(said + "[::]:8080"), name, assertion.Fail, "[::]:8080"},
		"the address, then a wildcard": {enforcerSaid(said + atAgent + "; " + said + "0.0.0.0:8080"), name,
			assertion.Fail, "0.0.0.0:8080"},
		"another port":           {enforcerSaid(said + agentAddress + ":9090"), name, assertion.Fail, ":9090"},
		"said nothing about it":  {enforcerSaid(""), name, assertion.Indeterminate, "not measured"},
		"a line that is not its": {enforcerSaid("listening for agents on nowhere"), name, assertion.Indeterminate, "not measured"},
		"its output was not read": {check.Probe{Kind: check.ListenerLog, Target: atAgent, From: "enforcer", Agent: agentAddress,
			Detail: "docker compose logs: exit status 1"}, name, assertion.Indeterminate, "did not run"},
	})
}

func runListenerCases(t *testing.T, cases map[string]listenerCase) {
	t.Helper()
	gateway := check.Probe{Target: "enforcer:8080", Ran: true, Reached: true}
	victim := check.Probe{Target: "victim-fs:8080", Ran: true, Detail: "lookup victim-fs: no such host"}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			probe := test.probe
			probe.Reached = strings.HasPrefix(probe.Detail, "{")
			checker := check.NetworkIsolation{Gateway: gateway, Victim: victim, Sealed: []check.Probe{probe}, Source: bootFile}
			results, err := checker.Run(context.Background(), records(nil, nil))
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != 3 {
				t.Fatalf("%d results, want 3", len(results))
			}
			got := results[2]
			if got.Check != test.check {
				t.Errorf("check %q, want %q", got.Check, test.check)
			}
			if got.Outcome != test.want || !strings.Contains(got.Got, test.got) {
				t.Errorf("%s: %s %q, want %s naming %q", got.Check, got.Outcome, got.Got, test.want, test.got)
			}
			if got.Source != bootFile || !strings.Contains(got.Want, probe.Target) {
				t.Errorf("result %+v does not name its record or what it wants", got)
			}
		})
	}
}
