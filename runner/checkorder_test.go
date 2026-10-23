package main

import (
	"context"
	"slices"
	"testing"
)

// The order of a run's checks is what a junit diff between two builds lines
// up, so a flow that reaches its subjects another way must still grade them in
// the order written out here.
func TestEveryKindOfRunReportsItsChecksInOneOrder(t *testing.T) {
	network := []string{
		"network-isolation/gateway-reachable",
		"network-isolation/victim-unreachable",
		"network-isolation/sealed/collector:4318",
		"network-isolation/sealed/enforcer:8081",
		"network-isolation/listener-closed-to/victim-fs",
		"network-isolation/agent-address-unreachable-from/victim-fs",
		"network-isolation/listener-bound-to-agent-net",
	}
	plane := []string{
		"plane/version",
		"plane/image",
		"plane/drained",
		"evidence/enforcement-mode",
		"evidence/executed-digest",
		"evidence/completed-succeeded",
		"decisions/step-1",
		"trails/opened",
		"effects/victim-fs",
		"evidence/run-id",
		"evidence/chain-complete/r1",
		"evidence/policy-digest-present",
		"evidence/content-captured",
	}
	trace := []string{
		"trace/report",
		"trace/exit-code",
		"trace/finding/contract.lab-bank-account.bank-account-needs-approval",
	}
	reach := []string{"verifier-reach/no-route-out", "verifier-reach/no-default-route", "verifier-reach/no-proxy"}
	tests := []struct {
		name string
		lab  func(t *testing.T) (lab, string)
		want []string
	}{
		{
			name: "a trajectory",
			lab:  func(t *testing.T) (lab, string) { subject, _, scenario := enforcerLab(t); return subject, scenario },
			want: slices.Concat([]string{"boot/enforcer", "boot/victim-fs"}, network, []string{"trajectory/replayed"}, plane),
		},
		{
			name: "a trajectory whose trace is graded",
			lab: func(t *testing.T) (lab, string) {
				subject, _, scenario := traceLab(t, thisRunsTrace, true)
				return subject, scenario
			},
			want: slices.Concat([]string{"boot/enforcer", "boot/victim-fs"}, network, []string{"trajectory/replayed"}, trace, plane),
		},
		{
			name: "a probe of the victims",
			lab: func(t *testing.T) (lab, string) {
				subject, _, scenario := verifierUnderTest(t)
				return subject, scenario
			},
			want: slices.Concat([]string{"verifier-reach/victim-fs:8080"}, reach, []string{
				"verifier/step-1/pin",
				"verifier/step-1/exit-code",
				"verifier/step-2/report",
				"verifier/step-2/exit-code",
				"verifier/step-2/finding/lab.example.drift",
				"verifier/step-2/no-finding/lab.example.cache_scope",
				"verifier/token-kept-out",
				"boot/victim-crm",
				"boot/victim-fs",
				"boot/mailpit",
				"effects/victim-crm",
				"effects/victim-fs",
			}),
		},
		{
			name: "a scan",
			lab: func(t *testing.T) (lab, string) {
				subject, _, scenario, _ := scanUnderTest(t)
				return subject, scenario
			},
			want: slices.Concat(reach, []string{
				"verifier/step-1/report",
				"verifier/step-1/exit-code",
				"verifier/step-1/gate",
				"verifier/step-1/shortfall",
				"verifier/step-1/errors",
				"verifier/step-1/scope",
				"verifier/step-1/findings",
				"verifier/step-1/unverified",
				"verifier/step-1/waived",
				"verifier/token-kept-out",
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			subject, scenario := test.lab(t)
			graded, err := subject.execute(context.Background(), scenario)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			got := make([]string, 0, len(graded.Results))
			for _, result := range graded.Results {
				got = append(got, result.Check)
			}
			if !slices.Equal(got, test.want) {
				t.Errorf("checks in the order graded:\n%q\nwant:\n%q", got, test.want)
			}
		})
	}
}
