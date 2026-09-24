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

const evidenceFile = "reports/run-1/evidence.jsonl"

func TestDecisionsGradeEachStepFromTheTrail(t *testing.T) {
	tests := []struct {
		name      string
		expect    labspec.DecisionExpectation
		tolerated []int
		recorded  []evidence.Event
		want      assertion.Outcome
		detail    string
	}{
		{
			name:     "verdict and reason codes as expected",
			expect:   labspec.DecisionExpectation{Verdict: "DENY", ReasonCodesInclude: []string{"RULE_DENY"}},
			recorded: trail(decided{step: 1, requestID: "r1", verdict: "DENY", reasons: []string{"RULE_DENY"}, blocked: true}),
			want:     assertion.Pass,
		},
		{
			name:     "reason codes are containment, not equality",
			expect:   labspec.DecisionExpectation{Verdict: "DENY", ReasonCodesInclude: []string{"RULE_DENY"}},
			recorded: trail(decided{step: 1, requestID: "r1", verdict: "DENY", reasons: []string{"RULE_DENY", "TENANT_MISMATCH"}, blocked: true}),
			want:     assertion.Pass,
		},
		{
			name:     "obligations are containment",
			expect:   labspec.DecisionExpectation{Verdict: "ALLOW_WITH_OBLIGATIONS", ObligationsInclude: []string{"label_sensitive"}},
			recorded: trail(decided{step: 1, requestID: "r1", verdict: "ALLOW_WITH_OBLIGATIONS", obligations: []string{"label_sensitive", "notify"}}),
			want:     assertion.Pass,
		},
		{
			name:     "no trail at all is a failure, because the record that should exist does not",
			expect:   labspec.DecisionExpectation{Verdict: "DENY"},
			recorded: nil,
			want:     assertion.Fail,
			detail:   "never opened",
		},
		{
			name:     "a proposal with no decision on its request is a failure",
			expect:   labspec.DecisionExpectation{Verdict: "DENY"},
			recorded: trail(decided{step: 1, requestID: "r1", verdict: "DENY", blocked: true})[:1],
			want:     assertion.Fail,
			detail:   "no " + string(evidence.KindPolicyDecided),
		},
		{
			name:   "two proposals carrying one step id cannot be graded as one",
			expect: labspec.DecisionExpectation{Verdict: "DENY"},
			recorded: trail(
				decided{step: 1, requestID: "r1", verdict: "DENY", blocked: true},
				decided{step: 1, requestID: "r2", verdict: "ALLOW"},
			),
			want:   assertion.Fail,
			detail: "r1",
		},
		{
			name:     "a verdict other than the one expected",
			expect:   labspec.DecisionExpectation{Verdict: "DENY"},
			recorded: trail(decided{step: 1, requestID: "r1", verdict: "ALLOW"}),
			want:     assertion.Fail,
		},
		{
			name:     "a missing reason code",
			expect:   labspec.DecisionExpectation{Verdict: "DENY", ReasonCodesInclude: []string{"TOXIC_FLOW_SENSITIVE_TO_EXTERNAL"}},
			recorded: trail(decided{step: 1, requestID: "r1", verdict: "DENY", reasons: []string{"RULE_DENY"}, blocked: true}),
			want:     assertion.Fail,
			detail:   "TOXIC_FLOW_SENSITIVE_TO_EXTERNAL",
		},
		{
			name:     "a missing obligation",
			expect:   labspec.DecisionExpectation{Verdict: "ALLOW_WITH_OBLIGATIONS", ObligationsInclude: []string{"label_sensitive"}},
			recorded: trail(decided{step: 1, requestID: "r1", verdict: "ALLOW_WITH_OBLIGATIONS", obligations: []string{"notify"}}),
			want:     assertion.Fail,
			detail:   "label_sensitive",
		},
		{
			name:     "an untolerated INDETERMINATE is a failure",
			expect:   labspec.DecisionExpectation{Verdict: "DENY"},
			recorded: trail(decided{step: 1, requestID: "r1", verdict: "INDETERMINATE", reasons: []string{"POLICY_UNAVAILABLE"}, blocked: true}),
			want:     assertion.Fail,
		},
		{
			name:      "a tolerated INDETERMINATE passes and says the tolerance was used",
			expect:    labspec.DecisionExpectation{Verdict: "DENY", ReasonCodesInclude: []string{"RULE_DENY"}},
			tolerated: []int{1},
			recorded:  trail(decided{step: 1, requestID: "r1", verdict: "INDETERMINATE", reasons: []string{"POLICY_UNAVAILABLE"}, blocked: true}),
			want:      assertion.Pass,
			detail:    "tolerance",
		},
		{
			name:      "a tolerance does not excuse a verdict that is merely wrong",
			expect:    labspec.DecisionExpectation{Verdict: "DENY"},
			tolerated: []int{1},
			recorded:  trail(decided{step: 1, requestID: "r1", verdict: "ALLOW"}),
			want:      assertion.Fail,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := scenario(map[int]labspec.DecisionExpectation{1: test.expect})
			spec.Tolerance.AllowIndeterminateForSteps = test.tolerated
			checker := check.Decisions{Scenario: spec, Trajectory: sameTool(spec), EvidenceFile: evidenceFile}

			results, err := checker.Run(context.Background(), records(test.recorded, nil))
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("got %d results, want one per graded step", len(results))
			}
			if results[0].Outcome != test.want {
				t.Errorf("outcome %s, want %s (%+v)", results[0].Outcome, test.want, results[0])
			}
			if test.detail != "" && !strings.Contains(results[0].Detail, test.detail) {
				t.Errorf("detail %q does not name %q", results[0].Detail, test.detail)
			}
		})
	}
}

func TestDecisionsSourceNamesTheLineTheVerdictWasReadFrom(t *testing.T) {
	spec := scenario(map[int]labspec.DecisionExpectation{
		1: {Verdict: "ALLOW"},
		2: {Verdict: "DENY"},
	})
	recorded := trail(
		decided{step: 1, requestID: "r1", verdict: "ALLOW"},
		decided{step: 2, requestID: "r2", verdict: "DENY", blocked: true},
	)
	checker := check.Decisions{Scenario: spec, Trajectory: sameTool(spec), EvidenceFile: evidenceFile}

	results, err := checker.Run(context.Background(), records(recorded, nil))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want one per step", len(results))
	}
	// Step 1's trail runs to four events and step 2's is blocked after three,
	// so step 2's decision is the sixth line. DecodeJSONL refuses a blank line,
	// which is what makes an index a line number.
	want := []string{evidenceFile + ":2", evidenceFile + ":6"}
	for i, result := range results {
		if result.Source != want[i] {
			t.Errorf("result %d source is %q, want %q", i, result.Source, want[i])
		}
		if result.Outcome != assertion.Pass {
			t.Errorf("result %d is %s: %s", i, result.Outcome, result.Detail)
		}
	}
}

func TestDecisionsGradeEveryStepTheScenarioNames(t *testing.T) {
	spec := scenario(map[int]labspec.DecisionExpectation{1: {Verdict: "ALLOW"}, 2: {Verdict: "DENY"}, 3: {Verdict: "DENY"}})
	recorded := trail(decided{step: 1, requestID: "r1", verdict: "ALLOW"})
	checker := check.Decisions{Scenario: spec, Trajectory: sameTool(spec), EvidenceFile: evidenceFile}

	results, err := checker.Run(context.Background(), records(recorded, nil))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results for a scenario naming three steps", len(results))
	}
	report := assertion.Run(context.Background(), records(recorded, nil), checker)
	if report.Outcome() != assertion.Fail {
		t.Errorf("a run missing two of three decisions reported %s", report.Outcome())
	}
}

// A step is graded from the record of this run and never from a record that
// happens to be in the file. An envelope stamped with another run's identifier
// claims a step of that run, and reading it as this one's would grade a run on
// whatever the last one left behind.
func TestDecisionsDoNotGradeAStepFromAnotherRunsTrail(t *testing.T) {
	spec := scenario(map[int]labspec.DecisionExpectation{1: {Verdict: "ALLOW"}})
	yesterday := trail(decided{step: 1, requestID: "r1", verdict: "ALLOW", runID: "run-yesterday"})
	checker := check.Decisions{Scenario: spec, Trajectory: sameTool(spec), EvidenceFile: evidenceFile}

	results, err := checker.Run(context.Background(), records(yesterday, nil))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want one per graded step", len(results))
	}
	if results[0].Outcome != assertion.Fail {
		t.Errorf("a step decided in another run was graded %s: %+v", results[0].Outcome, results[0])
	}
}
