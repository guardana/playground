package main

import (
	"slices"
	"testing"

	"github.com/guardana/playground/drivers/control"
	"github.com/guardana/playground/drivers/guardana"
	"github.com/guardana/playground/internal/labspec"
)

func TestEachRoleIsPickedFromTheScenariosOwnKeys(t *testing.T) {
	probe := []labspec.VerifierStep{{Probe: &labspec.ProbeStep{Server: "victim-fs"}}}
	tests := []struct {
		name                 string
		spec                 labspec.Scenario
		gate, prober, grader bool
		subjects             int
		down                 []string
	}{
		{"a gateway alone", labspec.Scenario{Profile: []string{"core"}, Gateway: &labspec.Gateway{}},
			true, false, false, 1, []string{"core"}},
		{"a gateway and a trace", labspec.Scenario{Profile: []string{"core"}, Gateway: &labspec.Gateway{}, Trace: &labspec.Trace{}},
			true, false, true, 2, []string{"core"}},
		{"verifier steps", labspec.Scenario{Profile: []string{"verifier"}, Verifier: probe},
			false, true, false, 1, []string{"verifier", "verifier-pin"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chosen := lab{}.pick(test.spec)
			_, gate := chosen.gate.(*control.Driver)
			_, prober := chosen.prober.(*guardana.Driver)
			_, grader := chosen.grader.(*guardana.Driver)
			if gate != test.gate || prober != test.prober || grader != test.grader {
				t.Errorf("picked gate %t, prober %t, grader %t; want %t, %t, %t", gate, prober, grader, test.gate, test.prober, test.grader)
			}
			if len(chosen.subjects) != test.subjects {
				t.Errorf("picked %d subjects, want %d", len(chosen.subjects), test.subjects)
			}
			if down := chosen.downProfiles(test.spec); !slices.Equal(down, test.down) {
				t.Errorf("down profiles %q, want %q", down, test.down)
			}
		})
	}
}

func TestBootLeavesEveryOneShotToTheRunner(t *testing.T) {
	want := []string{"scripted-agent", "verifier", "trace-verifier", "scan-verifier"}
	if got := (lab{}).oneShots(); !slices.Equal(got, want) {
		t.Errorf("one-shots %q, want %q", got, want)
	}
}
