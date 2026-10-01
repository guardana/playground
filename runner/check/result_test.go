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

// aborted is one trail whose call the adapter did not send: it closes
// ACTION_FAILED with the result status given, or with no result at all.
func aborted(status string) []evidence.Event {
	events := trail(decided{step: 1, requestID: "r1", verdict: "ALLOW_WITH_OBLIGATIONS"})
	closing := &events[len(events)-1]
	closing.Kind = evidence.KindActionFailed
	if status != "" {
		closing.Result = &evidence.ActionResult{Status: status}
	}
	return events
}

func resultStep(status string) labspec.Scenario {
	return scenario(map[int]labspec.DecisionExpectation{1: {
		Verdict: "ALLOW_WITH_OBLIGATIONS",
		Trail:   []string{"ACTION_PROPOSED", "POLICY_DECIDED", "ACTION_STARTED", "ACTION_FAILED"},
		Result:  &labspec.ResultExpectation{Status: status},
	}})
}

func TestAStepsClosingResultIsGraded(t *testing.T) {
	for name, tc := range map[string]struct {
		recorded string
		want     assertion.Outcome
		detail   string
	}{
		"the stated status":                     {"RESULT_STATUS_BLOCKED", assertion.Pass, ""},
		"another status":                        {"RESULT_STATUS_TIMEOUT", assertion.Fail, "TIMEOUT"},
		"a closing record that holds no result": {"", assertion.Fail, "no result"},
	} {
		t.Run(name, func(t *testing.T) {
			spec := resultStep("BLOCKED")
			results, err := check.Decisions{Scenario: spec, Trajectory: sameTool(spec), EvidenceFile: "evidence.jsonl"}.
				Run(context.Background(), records(aborted(tc.recorded), nil))
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != 1 || results[0].Outcome != tc.want {
				t.Fatalf("results = %+v, want one %s", results, tc.want)
			}
			if !strings.Contains(results[0].Detail, tc.detail) {
				t.Errorf("detail = %q, want it to name %q", results[0].Detail, tc.detail)
			}
		})
	}
}

func TestAStepsResultNeedsOneClosingRecord(t *testing.T) {
	spec := resultStep("SUCCESS")
	spec.Expect.Decisions[1] = labspec.DecisionExpectation{
		Verdict: "ALLOW", Result: &labspec.ResultExpectation{Status: "SUCCESS"},
	}
	blocked := trail(decided{step: 1, requestID: "r1", verdict: "ALLOW", blocked: true})
	results, err := check.Decisions{Scenario: spec, Trajectory: sameTool(spec), EvidenceFile: "evidence.jsonl"}.
		Run(context.Background(), records(blocked, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Outcome != assertion.Fail {
		t.Fatalf("a trail that never closed passed a stated result: %+v", results)
	}
}
