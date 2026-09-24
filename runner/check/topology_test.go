package check_test

import (
	"context"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/runner/check"
)

const bootFile = "reports/run-1/boot.json"

func TestBootGradesEveryServiceTheRunnerBroughtUp(t *testing.T) {
	tests := []struct {
		name    string
		boot    assertion.Boot
		results int
		want    assertion.Outcome
		detail  string
	}{
		{
			name: "every service running",
			boot: assertion.Boot{Profile: "core", Services: []assertion.Service{
				{Name: "victim-fs", Running: true},
				{Name: "stub-gateway", Running: true},
			}},
			results: 2,
			want:    assertion.Pass,
		},
		{
			name: "a service that did not start, and the last thing it said",
			boot: assertion.Boot{Profile: "core", Services: []assertion.Service{
				{Name: "victim-fs", Running: false, Detail: "exited (1): listen tcp :8080: address already in use"},
			}},
			results: 1,
			want:    assertion.Fail,
			detail:  "address already in use",
		},
		{
			name:    "a boot naming no service observed nothing",
			boot:    assertion.Boot{Profile: "core"},
			results: 1,
			want:    assertion.Indeterminate,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			given := records(nil, nil)
			given.Boot = test.boot
			results, err := check.Boot{Source: bootFile}.Run(context.Background(), given)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(results) != test.results {
				t.Fatalf("got %d results, want %d", len(results), test.results)
			}
			if results[0].Outcome != test.want {
				t.Errorf("outcome %s, want %s (%+v)", results[0].Outcome, test.want, results[0])
			}
			if test.detail != "" && !strings.Contains(results[0].Detail, test.detail) {
				t.Errorf("detail %q does not name %q", results[0].Detail, test.detail)
			}
			if results[0].Source != bootFile {
				t.Errorf("source is %q, want %q", results[0].Source, bootFile)
			}
		})
	}
}

func TestNetworkIsolationReadsBothProbes(t *testing.T) {
	reachedGateway := check.Probe{Target: "stub-gateway:8080", Ran: true, Reached: true}
	noRouteToVictim := check.Probe{
		Target: "victim-fs:8080", Ran: true,
		Detail: "dial tcp: lookup victim-fs: no such host",
	}

	tests := []struct {
		name    string
		gateway check.Probe
		victim  check.Probe
		want    []assertion.Outcome
	}{
		{
			name:    "the gateway answered and the victim had no route",
			gateway: reachedGateway,
			victim:  noRouteToVictim,
			want:    []assertion.Outcome{assertion.Pass, assertion.Pass},
		},
		{
			name:    "the victim answered, so the networks are joined",
			gateway: reachedGateway,
			victim:  check.Probe{Target: "victim-fs:8080", Ran: true, Reached: true},
			want:    []assertion.Outcome{assertion.Pass, assertion.Fail},
		},
		{
			name:    "the gateway had no route, so nothing could have been asked of it",
			gateway: check.Probe{Target: "stub-gateway:8080", Ran: true, Detail: "connection refused"},
			victim:  noRouteToVictim,
			want:    []assertion.Outcome{assertion.Fail, assertion.Pass},
		},
		{
			name:    "a probe that never ran establishes nothing about the topology",
			gateway: reachedGateway,
			victim:  check.Probe{Target: "victim-fs:8080", Detail: "the agent image could not be run"},
			want:    []assertion.Outcome{assertion.Pass, assertion.Indeterminate},
		},
		{
			name:    "neither probe ran",
			gateway: check.Probe{Target: "stub-gateway:8080"},
			victim:  check.Probe{Target: "victim-fs:8080"},
			want:    []assertion.Outcome{assertion.Indeterminate, assertion.Indeterminate},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checker := check.NetworkIsolation{Gateway: test.gateway, Victim: test.victim, Source: bootFile}
			results, err := checker.Run(context.Background(), records(nil, nil))
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(results) != 2 {
				t.Fatalf("got %d results, want one per probe", len(results))
			}
			for i, got := range results {
				if got.Outcome != test.want[i] {
					t.Errorf("result %d (%s) is %s, want %s", i, got.Check, got.Outcome, test.want[i])
				}
			}
		})
	}
}

// A probe that failed because the topology is right and a probe that failed
// because it never ran are the same exit code and different facts.
func TestNetworkIsolationCarriesTheReasonAProbeGave(t *testing.T) {
	checker := check.NetworkIsolation{
		Gateway: check.Probe{Target: "stub-gateway:8080", Ran: true, Reached: true},
		Victim:  check.Probe{Target: "victim-fs:8080", Ran: true, Detail: "lookup victim-fs: no such host"},
		Source:  bootFile,
	}
	results, err := checker.Run(context.Background(), records(nil, nil))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(results[1].Got, "no such host") {
		t.Errorf("the victim probe result does not carry the reason it got: %+v", results[1])
	}
}

func TestTheVerifierReachesWhatItProbesAndNothingOutside(t *testing.T) {
	reached := check.Probe{Target: "victim-fs:8080", Ran: true, Reached: true}
	noRoute := check.Probe{Target: "192.0.2.1:443", Ran: true, Detail: "Network is unreachable"}
	noDefault := check.Probe{Target: "a default route", Ran: true, Detail: "none"}
	for name, test := range map[string]struct {
		server, outside, routes check.Probe
		red                     string
		outcome                 assertion.Outcome
	}{
		"sealed": {reached, noRoute, noDefault, "", assertion.Pass},
		"a route out": {reached, check.Probe{Target: "192.0.2.1:443", Ran: true, Reached: true}, noDefault,
			"verifier-reach/no-route-out", assertion.Fail},
		"an unreached probe": {check.Probe{Target: "victim-fs:8080", Ran: true}, noRoute, noDefault,
			"verifier-reach/victim-fs:8080", assertion.Fail},
		"a dial that told nothing": {reached, check.Probe{Target: "192.0.2.1:443", Detail: "timed out"}, noDefault,
			"verifier-reach/no-route-out", assertion.Indeterminate},
		"an unreachable dial beside a default route": {reached, noRoute,
			check.Probe{Target: "a default route", Ran: true, Reached: true, Detail: "ipv4 via eth0"},
			"verifier-reach/no-default-route", assertion.Fail},
		"routing tables not read": {reached, noRoute, check.Probe{Target: "a default route", Detail: "no such file"},
			"verifier-reach/no-default-route", assertion.Indeterminate},
	} {
		t.Run(name, func(t *testing.T) {
			results, err := check.VerifierReach{Servers: []check.Probe{test.server}, Outside: test.outside, Routes: test.routes}.
				Run(context.Background(), assertion.Records{})
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != 3 {
				t.Fatalf("%d results, want 3", len(results))
			}
			for _, result := range results {
				want := assertion.Pass
				if result.Check == test.red {
					want = test.outcome
				}
				if result.Outcome != want {
					t.Errorf("%s = %s, want %s", result.Check, result.Outcome, want)
				}
			}
		})
	}
}
