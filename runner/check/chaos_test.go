package check_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

// oneCallTo is a trajectory of one fs.list on victim-fs, its scenario naming
// one latency toxic on that victim, and the trail of that call closing with a
// result that took took.
func oneCallTo(took time.Duration) (check.Chaos, assertion.Records) {
	spec := labspec.Scenario{
		Chaos:  []labspec.Fault{{Toxic: &labspec.Toxic{Victim: "victim-fs", Type: labspec.ToxicLatency}}},
		Expect: labspec.Expect{Decisions: map[int]labspec.DecisionExpectation{1: {Verdict: "ALLOW"}}},
	}
	trajectory := labspec.Trajectory{Steps: []labspec.Step{{Call: labspec.Call{Server: "victim-fs", Tool: "fs.list"}}}}
	started := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	ended := started.Add(took)
	events := []evidence.Event{
		{EventID: "e1", Kind: evidence.KindActionProposed, RequestID: "r1", Proposed: &evidence.ActionEnvelope{
			RequestID: "r1", Action: &evidence.Action{Name: "fs.list"}}},
		{EventID: "e2", Kind: evidence.KindActionCompleted, RequestID: "r1", PrevEventID: "e1",
			Result: &evidence.ActionResult{StartedAt: &started, EndedAt: &ended}},
	}
	graded := check.Chaos{
		Scenario: spec, Trajectory: trajectory, Source: "chaos.log",
		Faults: []check.ChaosFault{{
			Name: "latency 1.5s on victim-fs", Victim: "victim-fs", Latency: 1500 * time.Millisecond,
			Applied: true, Held: true, Lifted: true,
		}},
	}
	return graded, assertion.Records{RunID: thisRun, Evidence: events}
}

func chaosOutcomes(t *testing.T, graded check.Chaos, records assertion.Records) []assertion.Result {
	t.Helper()
	results, err := graded.Run(context.Background(), records)
	if err != nil {
		t.Fatal(err)
	}
	return results
}

func TestAFaultAppliedHeldAndLiftedOnTheSlowedPathPasses(t *testing.T) {
	graded, records := oneCallTo(1600 * time.Millisecond)
	results := chaosOutcomes(t, graded, records)
	if len(results) != 1 || results[0].Outcome != assertion.Pass {
		t.Fatalf("results = %+v", results)
	}
}

func TestAFaultNotShownInPlaceFails(t *testing.T) {
	for name, change := range map[string]func(*check.ChaosFault){
		"not applied":   func(f *check.ChaosFault) { f.Applied = false },
		"not read back": func(f *check.ChaosFault) { f.Held = false },
		"not lifted":    func(f *check.ChaosFault) { f.Lifted = false },
	} {
		t.Run(name, func(t *testing.T) {
			graded, records := oneCallTo(1600 * time.Millisecond)
			change(&graded.Faults[0])
			results := chaosOutcomes(t, graded, records)
			if results[0].Outcome != assertion.Fail || !strings.Contains(results[0].Got, name) {
				t.Errorf("a fault %s was %s: %s", name, results[0].Outcome, results[0].Got)
			}
		})
	}
}

// A call that came back faster than the latency did not go through the
// proxy, whatever the proxy says about its toxics.
func TestALatencyTheTrailDoesNotShowFails(t *testing.T) {
	graded, records := oneCallTo(40 * time.Millisecond)
	if results := chaosOutcomes(t, graded, records); results[0].Outcome != assertion.Fail ||
		!strings.Contains(results[0].Got, "under the latency") {
		t.Errorf("a call faster than the latency was %s: %s", results[0].Outcome, results[0].Got)
	}
	graded.Trajectory.Steps[0].Call.Server = "victim-crm"
	if results := chaosOutcomes(t, graded, records); results[0].Outcome != assertion.Fail ||
		!strings.Contains(results[0].Got, "nothing went through") {
		t.Errorf("a latency no call crossed was %s: %s", results[0].Outcome, results[0].Got)
	}
}

func TestFaultsTheRunnerDidNotRecordAreIndeterminate(t *testing.T) {
	graded, records := oneCallTo(1600 * time.Millisecond)
	graded.Faults = nil
	if results := chaosOutcomes(t, graded, records); len(results) != 1 || results[0].Outcome != assertion.Indeterminate {
		t.Errorf("unrecorded faults were graded %+v", results)
	}
}

// A hang is shown on the trail by the call it held: closed as timed out, no
// sooner than the scenario's call timeout and within a second after it.
func TestAHangIsTheCallTimeoutOnTheTrail(t *testing.T) {
	const bound = 3 * time.Second
	for name, tc := range map[string]struct {
		status string
		took   time.Duration
		bound  time.Duration
		want   assertion.Outcome
		says   string
	}{
		"timed out at the bound":        {"RESULT_STATUS_TIMEOUT", bound, bound, assertion.Pass, "read:"},
		"timed out just after":          {"RESULT_STATUS_TIMEOUT", 3900 * time.Millisecond, bound, assertion.Pass, "read:"},
		"failed another way":            {"RESULT_STATUS_UNKNOWN", 3001 * time.Millisecond, bound, assertion.Fail, "not RESULT_STATUS_TIMEOUT"},
		"closed with no status":         {"", 3001 * time.Millisecond, bound, assertion.Fail, "closed nothing"},
		"cut off before the bound":      {"RESULT_STATUS_TIMEOUT", 2900 * time.Millisecond, bound, assertion.Fail, "took 2.9s"},
		"cut off a second past the end": {"RESULT_STATUS_TIMEOUT", 4 * time.Second, bound, assertion.Fail, "took 4s"},
		"no bound written down":         {"RESULT_STATUS_TIMEOUT", 3001 * time.Millisecond, 0, assertion.Fail, "no upstream.call_timeout"},
	} {
		t.Run(name, func(t *testing.T) {
			graded, records := oneCallTo(tc.took)
			graded.Faults[0].Latency, graded.Faults[0].Hang, graded.Faults[0].CallTimeout = 0, true, tc.bound
			records.Evidence[1].Kind, records.Evidence[1].Result.Status = evidence.KindActionFailed, tc.status
			results := chaosOutcomes(t, graded, records)
			if results[0].Outcome != tc.want || !strings.Contains(results[0].Got, tc.says) {
				t.Errorf("got %s: %s", results[0].Outcome, results[0].Got)
			}
		})
	}
}

// A second listing is not undone, so it is graded without a lift and says so.
func TestAnUnliftableFaultIsNotGradedOnALift(t *testing.T) {
	graded, records := oneCallTo(1600 * time.Millisecond)
	graded.Faults[0] = check.ChaosFault{Name: "a second listing", Applied: true, Held: true, Unliftable: true, Detail: "fs.read described anew"}
	results := chaosOutcomes(t, graded, records)
	if results[0].Outcome != assertion.Pass || !strings.Contains(results[0].Want, "lifting does not apply") ||
		results[0].Got != "read: fs.read described anew" {
		t.Errorf("an unliftable fault was graded %+v", results[0])
	}
}
