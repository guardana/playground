package check_test

import (
	"context"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/runner/check"
)

const (
	authorizedDigest = "sha256:" + "1111111111111111111111111111111111111111111111111111111111111111"
	otherDigest      = "sha256:" + "2222222222222222222222222222222222222222222222222222222222222222"
)

// enforced is one run the enforcer wrote under mode: an allowed call that
// completed with executed as its executed digest, and a blocked one.
func enforced(mode, executed string) []evidence.Event {
	events := trail(
		decided{step: 1, requestID: "r1", verdict: "ALLOW"},
		decided{step: 2, requestID: "r2", verdict: "DENY", blocked: true},
	)
	for i := range events {
		events[i].EnforcementMode = mode
		switch events[i].Kind {
		case evidence.KindPolicyDecided:
			events[i].Decision.ActionDigest = authorizedDigest
		case evidence.KindActionCompleted:
			events[i].Result = &evidence.ActionResult{ExecutedActionDigest: executed}
		}
	}
	return events
}

func runClaims(t *testing.T, mode string, events []evidence.Event) map[string]assertion.Result {
	t.Helper()
	spec := scenario(nil)
	spec.EnforcementMode = mode
	results, err := check.TrailClaims{Scenario: spec, EvidenceFile: evidenceFile}.Run(context.Background(), records(events, nil))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	found := map[string]assertion.Result{}
	for _, result := range results {
		found[result.Check] = result
	}
	return found
}

func TestTrailClaimsHoldEveryEventToTheScenariosMode(t *testing.T) {
	for name, test := range map[string]struct {
		scenarioMode string
		events       []evidence.Event
		want         assertion.Outcome
		detail       string
	}{
		"every event under the stated mode": {"observe", enforced("ENFORCEMENT_MODE_OBSERVE", authorizedDigest), assertion.Pass, ""},
		"enforced where observe was stated": {"observe", enforced("ENFORCEMENT_MODE_ENFORCE", authorizedDigest), assertion.Fail, "line 1 ENFORCEMENT_MODE_ENFORCE"},
		"no mode on the events":             {"enforce", enforced("", authorizedDigest), assertion.Fail, "line 1 nothing"},
		"no events to read":                 {"enforce", nil, assertion.Indeterminate, ""},
	} {
		t.Run(name, func(t *testing.T) {
			got := runClaims(t, test.scenarioMode, test.events)["evidence/enforcement-mode"]
			if got.Outcome != test.want || !strings.Contains(got.Detail, test.detail) {
				t.Errorf("got %s %q, want %s naming %q", got.Outcome, got.Detail, test.want, test.detail)
			}
		})
	}
}

// One event out of step is enough: a mode that changed mid-run is a run whose
// calls were not all enforced as the scenario states.
func TestTrailClaimsCatchOneEventUnderAnotherMode(t *testing.T) {
	events := enforced("ENFORCEMENT_MODE_LOCKDOWN", authorizedDigest)
	events[len(events)-1].EnforcementMode = "ENFORCEMENT_MODE_ENFORCE"
	got := runClaims(t, "lockdown", events)["evidence/enforcement-mode"]
	if got.Outcome != assertion.Fail || got.Source != evidenceFile+":7" {
		t.Errorf("got %s from %s, want a failure at line 7", got.Outcome, got.Source)
	}
}

func TestTrailClaimsCompareWhatRanWithWhatWasDecided(t *testing.T) {
	for name, test := range map[string]struct {
		executed string
		want     assertion.Outcome
		detail   string
	}{
		"the authorized bytes ran":    {authorizedDigest, assertion.Pass, ""},
		"other bytes ran":             {otherDigest, assertion.Fail, "executed " + otherDigest},
		"no executed digest at all":   {"", assertion.Fail, "comparison did not run"},
		"a digest nothing decided on": {authorizedDigest, assertion.Fail, "authorized nothing"},
	} {
		t.Run(name, func(t *testing.T) {
			events := enforced("ENFORCEMENT_MODE_ENFORCE", test.executed)
			if strings.HasPrefix(name, "a digest nothing") {
				events[1].Decision.ActionDigest = ""
			}
			got := runClaims(t, "enforce", events)["evidence/executed-digest"]
			if got.Outcome != test.want || !strings.Contains(got.Detail, test.detail) {
				t.Errorf("got %s %q, want %s naming %q", got.Outcome, got.Detail, test.want, test.detail)
			}
		})
	}
}

// A completion is read against its own request's decision, never another's:
// here the first decision in the file is a blocked call's, over other bytes.
func TestTrailClaimsReadTheDecisionOnTheCompletionsOwnRequest(t *testing.T) {
	events := trail(
		decided{step: 1, requestID: "r2", verdict: "DENY", blocked: true},
		decided{step: 2, requestID: "r1", verdict: "ALLOW"},
	)
	for i := range events {
		events[i].EnforcementMode = "ENFORCEMENT_MODE_ENFORCE"
		switch {
		case events[i].Kind == evidence.KindPolicyDecided && events[i].RequestID == "r2":
			events[i].Decision.ActionDigest = otherDigest
		case events[i].Kind == evidence.KindPolicyDecided:
			events[i].Decision.ActionDigest = authorizedDigest
		case events[i].Kind == evidence.KindActionCompleted:
			events[i].Result = &evidence.ActionResult{ExecutedActionDigest: authorizedDigest}
		}
	}
	got := runClaims(t, "enforce", events)["evidence/executed-digest"]
	if got.Outcome != assertion.Pass {
		t.Errorf("got %s: %s", got.Outcome, got.Detail)
	}
}

// A run in which nothing ran leaves nothing to compare, and says nothing
// about it rather than passing a comparison nobody made.
func TestTrailClaimsCompareNothingWhenNothingRan(t *testing.T) {
	events := enforced("ENFORCEMENT_MODE_ENFORCE", authorizedDigest)[4:]
	found := runClaims(t, "enforce", events)
	if _, graded := found["evidence/executed-digest"]; graded {
		t.Errorf("a run with no completion was graded on its digests: %+v", found["evidence/executed-digest"])
	}
	if found["evidence/enforcement-mode"].Outcome != assertion.Pass {
		t.Errorf("mode: %+v", found["evidence/enforcement-mode"])
	}
}

// ACTION_STARTED carries no result at the pin; one that does is held to the
// same equality.
func TestTrailClaimsCompareAStartThatCarriesAResult(t *testing.T) {
	events := enforced("ENFORCEMENT_MODE_ENFORCE", authorizedDigest)
	events[2].Result = &evidence.ActionResult{ExecutedActionDigest: otherDigest}
	got := runClaims(t, "enforce", events)["evidence/executed-digest"]
	if got.Outcome != assertion.Fail || got.Got != "1 of 2 equal" {
		t.Errorf("got %s %q: %s", got.Outcome, got.Got, got.Detail)
	}
}
