package trailcheck_test

import (
	"context"
	"testing"
	"time"

	"github.com/guardana/playground/drivers/control/trailcheck"
	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
)

// A trail nobody read is not an empty trail. Over an empty one a step that
// opens no trail passes, the openings balance and a slowed path shows no call,
// so each check below would grade something; over a trail never read every one
// of them has established nothing.
func TestATrailNobodyReadIsIndeterminateForEveryCheckThatReadsIt(t *testing.T) {
	spec := scenario(map[int]labspec.DecisionExpectation{1: {Opens: labspec.OpensNone}})
	spec.Chaos = []labspec.Fault{{Toxic: &labspec.Toxic{Victim: "victim-fs", Type: labspec.ToxicLatency}}}
	trajectory := sameTool(spec)
	for holder, trail := range map[string]*trailcheck.Trail{"no holder": nil, "a holder nothing filled": {}} {
		for _, subject := range []assertion.Check{
			trailcheck.Decisions{Scenario: spec, Trajectory: trajectory, EvidenceFile: evidenceFile, Trail: trail},
			trailcheck.Trails{Scenario: spec, Trajectory: trajectory, EvidenceFile: evidenceFile, Trail: trail},
			trailcheck.TrailClaims{Scenario: spec, EvidenceFile: evidenceFile, Trail: trail},
			trailcheck.Evidence{Scenario: spec, EvidenceFile: evidenceFile, FreshTrail: true, Trail: trail},
			trailcheck.Chaos{
				Scenario: spec, Trajectory: trajectory, Source: "chaos.log", Trail: trail,
				Faults: []trailcheck.ChaosFault{{
					Name: "latency on victim-fs", Victim: "victim-fs", Latency: time.Second,
					Applied: true, Held: true, Lifted: true,
				}},
			},
		} {
			t.Run(holder+"/"+subject.ID(), func(t *testing.T) {
				report := assertion.Run(context.Background(), records(), subject)
				if len(report.Results) != 1 {
					t.Fatalf("%d results over a trail nobody read, want the one that says so: %+v", len(report.Results), report.Results)
				}
				if got := report.Results[0]; got.Outcome != assertion.Indeterminate {
					t.Errorf("%s is %s over a trail nobody read: %s", got.Check, got.Outcome, got.Got)
				}
			})
		}
	}
}
