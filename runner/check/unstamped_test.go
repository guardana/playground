package check_test

import (
	"context"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

func TestATrailIsAssertedToNameOneRunOnEveryRequest(t *testing.T) {
	run := func(events []evidence.Event) assertion.Result {
		results, err := check.Evidence{Scenario: heldSpec(), EvidenceFile: evidenceFile, FreshTrail: true}.
			Run(context.Background(), records(events, nil))
		if err != nil {
			t.Fatal(err)
		}
		return result(t, results, "evidence/run-id")
	}
	if result := run(heldThenResumed().events); result.Outcome != assertion.Pass {
		t.Errorf("a trail naming one run was %s: %s", result.Outcome, result.Detail)
	}
	mutants := map[string]func(events []evidence.Event){
		"another run":   func(events []evidence.Event) { events[4].RunID = "run-elsewhere" },
		"no run":        func(events []evidence.Event) { events[4].RunID = "" },
		"envelope run":  func(events []evidence.Event) { events[0].Proposed.Context = &evidence.RunContext{RunID: planeRun} },
		"lab run named": func(events []evidence.Event) { events[4].RunID = thisRun },
	}
	for name, mutate := range mutants {
		events := heldThenResumed().events
		mutate(events)
		if result := run(events); result.Outcome != assertion.Fail {
			t.Errorf("%s: the trail was %s: %s", name, result.Outcome, result.Detail)
		}
	}
}

// A call the enforcer refused before it computed a run, such as one to a tool
// nothing classifies, has no run: its proposal says so, and only then may its
// events name none. It is still this trail's, and pairs with its step.
func TestARequestDecidedWithNoRunNamesNone(t *testing.T) {
	build := func(tagged bool) []evidence.Event {
		refused := trail(decided{step: 1, requestID: "r0", verdict: "INDETERMINATE", blocked: true})
		for i := range refused {
			refused[i].RunID = ""
		}
		if tagged {
			refused[0].Proposed.Context = &evidence.RunContext{Tags: []string{"flow.v1.state=uncomputed"}}
		}
		if !tagged && len(refused[0].Proposed.Context.Tags) == 0 {
			t.Fatal("the untagged case carries no flow tags, so it cannot tell uncomputed from any other tag")
		}
		return append(refused, trail(decided{step: 2, requestID: "r1", verdict: "ALLOW"})...)
	}
	spec := scenario(map[int]labspec.DecisionExpectation{
		1: {Verdict: "INDETERMINATE"},
		2: {Verdict: "ALLOW"},
	})
	runID := func(events []evidence.Event) assertion.Result {
		results, err := check.Evidence{Scenario: spec, EvidenceFile: evidenceFile, FreshTrail: true}.
			Run(context.Background(), records(events, nil))
		if err != nil {
			t.Fatal(err)
		}
		return result(t, results, "evidence/run-id")
	}
	graded := grade(t, spec, build(true))
	graded["evidence/run-id"] = runID(build(true))
	for _, name := range []string{"evidence/run-id", "decisions/step-1", "decisions/step-2", "trails/opened"} {
		if graded[name].Outcome != assertion.Pass {
			t.Errorf("%s was %s: %s", name, graded[name].Outcome, graded[name].Detail)
		}
	}
	if got := runID(build(false)); got.Outcome != assertion.Fail {
		t.Errorf("a runless request whose proposal does not say so was %s: %s", got.Outcome, got.Detail)
	}
}

func TestEventsWithNoRequestAreGradedApartFromTheChains(t *testing.T) {
	spec := heldSpec()
	spec.Expect.Evidence.ChainComplete = true
	find := func(events []evidence.Event) (assertion.Result, int) {
		results, err := check.Evidence{Scenario: spec, EvidenceFile: evidenceFile, FreshTrail: true}.
			Run(context.Background(), records(events, nil))
		if err != nil {
			t.Fatal(err)
		}
		failed := 0
		var found assertion.Result
		for _, result := range results {
			if result.Check == "evidence/requestless" {
				found = result
			}
			if strings.HasPrefix(result.Check, "evidence/chain-complete") && result.Outcome != assertion.Pass {
				failed++
			}
		}
		return found, failed
	}
	reload := evidence.Event{EventID: "reload-1", Kind: evidence.KindPolicyReloaded, ProjectID: thisProject, TenantID: thisTenant}
	result, failedChains := find(append(heldThenResumed().events, reload))
	if result.Outcome != assertion.Pass || failedChains != 0 {
		t.Errorf("a reload with no request was %s with %d chains failed", result.Outcome, failedChains)
	}
	stray := reload
	stray.Kind = evidence.KindActionProposed
	if result, _ := find(append(heldThenResumed().events, stray)); result.Outcome != assertion.Fail {
		t.Errorf("a proposal with no request was %s", result.Outcome)
	}
}

func TestARequestlessEventNamesNoOtherRun(t *testing.T) {
	reload := evidence.Event{EventID: "reload-1", Kind: evidence.KindPolicyReloaded, ProjectID: thisProject, TenantID: thisTenant}
	run := func(events []evidence.Event) assertion.Result {
		results, err := check.Evidence{Scenario: heldSpec(), EvidenceFile: evidenceFile, FreshTrail: true}.
			Run(context.Background(), records(events, nil))
		if err != nil {
			t.Fatal(err)
		}
		return result(t, results, "evidence/run-id")
	}
	for runID, want := range map[string]assertion.Outcome{"": assertion.Pass, planeRun: assertion.Pass, "run-elsewhere": assertion.Fail} {
		stamped := reload
		stamped.RunID = runID
		if got := run(append(heldThenResumed().events, stamped)); got.Outcome != want {
			t.Errorf("a reload naming %q was %s, want %s: %s", runID, got.Outcome, want, got.Detail)
		}
	}
	if got := run([]evidence.Event{reload}); got.Outcome != assertion.Indeterminate {
		t.Errorf("a trail with no request was %s, want indeterminate: %s", got.Outcome, got.Detail)
	}
}

func TestATrailIsOnlyThisRunsWhenTheRunnerMadeItsDirectory(t *testing.T) {
	for _, fresh := range []bool{false, true} {
		results, err := check.Evidence{Scenario: heldSpec(), EvidenceFile: evidenceFile, FreshTrail: fresh}.
			Run(context.Background(), records(heldThenResumed().events, nil))
		if err != nil {
			t.Fatal(err)
		}
		want := map[bool]assertion.Outcome{false: assertion.Indeterminate, true: assertion.Pass}[fresh]
		if got := result(t, results, "evidence/run-id"); got.Outcome != want {
			t.Errorf("fresh=%t: the trail was %s, want %s: %s", fresh, got.Outcome, want, got.Detail)
		}
	}
}
