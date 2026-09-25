package check_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

func evidenceScenario(expect labspec.EvidenceExpectation) labspec.Scenario {
	spec := scenario(map[int]labspec.DecisionExpectation{1: {Verdict: "ALLOW"}})
	spec.Expect.Evidence = &expect
	return spec
}

func result(t *testing.T, results []assertion.Result, name string) assertion.Result {
	t.Helper()
	for _, candidate := range results {
		if candidate.Check == name {
			return candidate
		}
	}
	names := make([]string, 0, len(results))
	for _, candidate := range results {
		names = append(names, candidate.Check)
	}
	t.Fatalf("no result named %q; got %s", name, strings.Join(names, ", "))
	return assertion.Result{}
}

func TestEvidenceChainCompleteSeparatesBrokenFromIndeterminate(t *testing.T) {
	whole := trail(decided{step: 1, requestID: "r1", verdict: "ALLOW"})

	broken := trail(decided{step: 1, requestID: "r1", verdict: "ALLOW"})
	broken[3].PrevEventID = "an-event-that-is-not-the-one-before-it"

	unplaceable := trail(decided{step: 1, requestID: "r1", verdict: "ALLOW"})
	unplaceable = append(unplaceable, evidence.Event{
		EventID:     "r1-5",
		Kind:        "EVENT_KIND_FROM_A_LATER_VERSION",
		RequestID:   "r1",
		ProjectID:   thisProject,
		TenantID:    thisTenant,
		PrevEventID: "r1-4",
	})

	tests := []struct {
		name     string
		recorded []evidence.Event
		want     assertion.Outcome
	}{
		{"a whole trail", whole, assertion.Pass},
		{"a link that does not join is a definite defect", broken, assertion.Fail},
		{"a kind this reader cannot place establishes nothing", unplaceable, assertion.Indeterminate},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checker := check.Evidence{
				Scenario:     evidenceScenario(labspec.EvidenceExpectation{ChainComplete: true}),
				EvidenceFile: evidenceFile,
			}
			results, err := checker.Run(context.Background(), records(test.recorded, nil))
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			got := result(t, results, "evidence/chain-complete/r1")
			if got.Outcome != test.want {
				t.Errorf("outcome %s, want %s (%+v)", got.Outcome, test.want, got)
			}
		})
	}
}

func TestEvidenceChainCompleteIsNotAssertedWhenTheScenarioDoesNotAskForIt(t *testing.T) {
	broken := trail(decided{step: 1, requestID: "r1", verdict: "ALLOW"})
	broken[3].PrevEventID = "nothing"

	checker := check.Evidence{
		Scenario:     evidenceScenario(labspec.EvidenceExpectation{ChainComplete: false}),
		EvidenceFile: evidenceFile,
	}
	results, err := checker.Run(context.Background(), records(broken, nil))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, got := range results {
		if strings.HasPrefix(got.Check, "evidence/chain-complete") {
			t.Errorf("a scenario that did not ask for a complete chain was graded on one: %+v", got)
		}
	}
}

func TestEvidencePolicyDigestIsReadFromEveryDecision(t *testing.T) {
	tests := []struct {
		name     string
		recorded []evidence.Event
		want     assertion.Outcome
	}{
		{
			name:     "every decision carries a digest",
			recorded: trail(decided{step: 1, requestID: "r1", verdict: "ALLOW"}),
			want:     assertion.Pass,
		},
		{
			name:     "a decision with no digest",
			recorded: trail(decided{step: 1, requestID: "r1", verdict: "ALLOW", digest: " "}),
			want:     assertion.Fail,
		},
		{
			name:     "no decision to read a digest from establishes nothing",
			recorded: trail(decided{step: 1, requestID: "r1", verdict: "ALLOW"})[:1],
			want:     assertion.Indeterminate,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checker := check.Evidence{
				Scenario:     evidenceScenario(labspec.EvidenceExpectation{PolicyDigestPresent: true}),
				EvidenceFile: evidenceFile,
			}
			results, err := checker.Run(context.Background(), records(test.recorded, nil))
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			got := result(t, results, "evidence/policy-digest-present")
			if got.Outcome != test.want {
				t.Errorf("outcome %s, want %s (%+v)", got.Outcome, test.want, got)
			}
		})
	}
}

func TestEvidenceContentCaptureIsAssertedInBothDirections(t *testing.T) {
	clean := trail(decided{step: 1, requestID: "r1", verdict: "ALLOW"})
	leaking := trail(decided{step: 1, requestID: "r1", verdict: "ALLOW", preview: "CANARY-0001 was here"})

	tests := []struct {
		name     string
		captured bool
		recorded []evidence.Event
		want     assertion.Outcome
	}{
		{"no preview where none is expected", false, clean, assertion.Pass},
		{"a preview where none is expected", false, leaking, assertion.Fail},
		{"a preview where one is expected", true, leaking, assertion.Pass},
		{"no preview where one is expected", true, clean, assertion.Fail},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checker := check.Evidence{
				Scenario:     evidenceScenario(labspec.EvidenceExpectation{ContentCaptured: test.captured}),
				EvidenceFile: evidenceFile,
			}
			results, err := checker.Run(context.Background(), records(test.recorded, nil))
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			got := result(t, results, "evidence/content-captured")
			if got.Outcome != test.want {
				t.Errorf("outcome %s, want %s (%+v)", got.Outcome, test.want, got)
			}
		})
	}
}

// The trap this check exists to avoid: "no envelope carries a preview" is true
// of a trail with no envelopes in it, and reading that as a pass would let a
// run that recorded nothing report that the privacy default held. A trail with
// nothing in it is a failure — the enforcement plane recorded no decision —
// and never a pass.
func TestEvidenceFailsOnATrailWithNothingInIt(t *testing.T) {
	checker := check.Evidence{
		Scenario: evidenceScenario(labspec.EvidenceExpectation{
			ChainComplete:       true,
			PolicyDigestPresent: true,
			ContentCaptured:     false,
		}),
		EvidenceFile: evidenceFile,
	}
	results, err := checker.Run(context.Background(), records(nil, nil))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("the check reported nothing at all")
	}
	for _, got := range results {
		if got.Outcome != assertion.Fail {
			t.Errorf("%s is %s over an empty trail, want fail", got.Check, got.Outcome)
		}
	}
	report := assertion.Run(context.Background(), records(nil, nil), checker)
	if report.Outcome() != assertion.Fail {
		t.Errorf("a run over an empty trail reported %s", report.Outcome())
	}
}

// An empty trail and a trail nobody could read are two facts. Both are red, so
// nothing goes green either way — but the reader sent to an empty trail opens
// the file, finds it full, and has to start the search again somewhere else.
func TestEvidenceSeparatesATrailItCouldNotReadFromAnEmptyOne(t *testing.T) {
	unreadable := check.Evidence{
		Scenario:     evidenceScenario(labspec.EvidenceExpectation{ChainComplete: true}),
		EvidenceFile: evidenceFile,
		ReadError:    errors.New("line 3: evidence: malformed line: unexpected end of JSON input"),
	}
	results, err := unreadable.Run(context.Background(), records(nil, nil))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want the one fact the check established", len(results))
	}
	got := results[0]
	if got.Outcome != assertion.Indeterminate {
		t.Errorf("a trail that could not be read is %s, want indeterminate (%+v)", got.Outcome, got)
	}
	if !strings.Contains(got.Detail, "malformed line") {
		t.Errorf("the result drops the read failure: %+v", got)
	}
	if strings.Contains(got.Got+got.Detail, "empty") {
		t.Errorf("a trail that could not be read is reported as an empty one: %+v", got)
	}
	if got.Source != evidenceFile {
		t.Errorf("source is %q, want the file the reader has to open", got.Source)
	}
}
