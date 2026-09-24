package check_test

import (
	"context"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/runner/check"
)

func TestAnUnstampedTrailIsAssertedToNameNoRun(t *testing.T) {
	events := heldThenResumed().events
	spec := heldSpec()
	run := func(events []evidence.Event) assertion.Result {
		results, err := check.Evidence{Scenario: spec, EvidenceFile: evidenceFile, Unstamped: true, FreshTrail: true}.
			Run(context.Background(), records(events, nil))
		if err != nil {
			t.Fatal(err)
		}
		for _, result := range results {
			if result.Check == "evidence/run-id" {
				return result
			}
		}
		t.Fatal("no evidence/run-id result")
		return assertion.Result{}
	}
	if result := run(events); result.Outcome != assertion.Pass {
		t.Errorf("an unstamped trail was %s: %s", result.Outcome, result.Detail)
	}
	events[4].RunID = "run-elsewhere"
	if result := run(events); result.Outcome != assertion.Fail || !strings.Contains(result.Detail, "run-elsewhere") {
		t.Errorf("an event naming a run in an unstamped trail was %s: %s", result.Outcome, result.Detail)
	}
}

func TestEventsWithNoRequestAreGradedApartFromTheChains(t *testing.T) {
	spec := heldSpec()
	spec.Expect.Evidence.ChainComplete = true
	find := func(events []evidence.Event) (assertion.Result, int) {
		results, err := check.Evidence{Scenario: spec, EvidenceFile: evidenceFile, Unstamped: true, FreshTrail: true}.
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

func TestAnUnstampedTrailIsOnlyThisRunsWhenTheRunnerMadeItsDirectory(t *testing.T) {
	for _, fresh := range []bool{false, true} {
		results, err := check.Evidence{Scenario: heldSpec(), EvidenceFile: evidenceFile, Unstamped: true, FreshTrail: fresh}.
			Run(context.Background(), records(heldThenResumed().events, nil))
		if err != nil {
			t.Fatal(err)
		}
		want := map[bool]assertion.Outcome{false: assertion.Indeterminate, true: assertion.Pass}[fresh]
		if got := result(t, results, "evidence/run-id"); got.Outcome != want {
			t.Errorf("fresh=%t: an unstamped trail was %s, want %s: %s", fresh, got.Outcome, want, got.Detail)
		}
	}
}
