package labspec_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

const heldStepThree = "3: { resumes: 1, trail: [ACTION_PROPOSED, POLICY_DECIDED, APPROVAL_REQUESTED, APPROVAL_DECIDED, ACTION_STARTED, ACTION_COMPLETED] }"

const heldStepOne = "1: { verdict: REQUIRE_APPROVAL, trail: [ACTION_PROPOSED, POLICY_DECIDED, APPROVAL_REQUESTED, APPROVAL_DECIDED, ACTION_STARTED, ACTION_COMPLETED] }"

func TestAStepStatesItsClosingResult(t *testing.T) {
	body := strings.NewReplacer(
		heldStepOne, "1: { verdict: ALLOW_WITH_OBLIGATIONS, trail: [ACTION_PROPOSED, POLICY_DECIDED, ACTION_STARTED, ACTION_FAILED], result: { status: BLOCKED } }",
		heldStepThree, "3: { verdict: ALLOW }",
	).Replace(heldScenario)
	if !strings.Contains(body, "result: { status: BLOCKED }") || !strings.Contains(body, "3: { verdict: ALLOW }") {
		t.Fatal("the replacement did not make step 1 a closed trail nobody resumes")
	}
	scenario, trajectory := loadPair(t, body)
	if err := labspec.Validate(scenario, trajectory); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got := scenario.Expect.Decisions[1].Result; got == nil || got.Status != "BLOCKED" {
		t.Errorf("step 1 result = %+v, want status BLOCKED", got)
	}
}

func TestAResultNoRecordCanCarryIsRefused(t *testing.T) {
	for name, step := range map[string]string{
		"a status the wire does not have": "1: { verdict: ALLOW, trail: [ACTION_PROPOSED, POLICY_DECIDED, ACTION_STARTED, ACTION_FAILED], result: { status: DONE } }",
		"the wire's prefix kept":          "1: { verdict: ALLOW, trail: [ACTION_PROPOSED, POLICY_DECIDED, ACTION_STARTED, ACTION_FAILED], result: { status: RESULT_STATUS_BLOCKED } }",
		"no status":                       "1: { verdict: ALLOW, trail: [ACTION_PROPOSED, POLICY_DECIDED, ACTION_STARTED, ACTION_FAILED], result: {} }",
		"no trail to read it from":        "1: { verdict: ALLOW, result: { status: SUCCESS } }",
		"a trail that never closes":       "1: { verdict: DENY, trail: [ACTION_PROPOSED, POLICY_DECIDED, ACTION_BLOCKED], result: { status: BLOCKED } }",
		"success on a failed trail":       "1: { verdict: ALLOW, trail: [ACTION_PROPOSED, POLICY_DECIDED, ACTION_STARTED, ACTION_FAILED], result: { status: SUCCESS } }",
		"a failure on a completed trail":  "1: { verdict: ALLOW, trail: [ACTION_PROPOSED, POLICY_DECIDED, ACTION_STARTED, ACTION_COMPLETED], result: { status: FAILURE } }",
		"beside a block":                  "1: { verdict: DENY, blocked: { verdict: DENY }, trail: [ACTION_PROPOSED, POLICY_DECIDED, ACTION_BLOCKED, ACTION_FAILED], result: { status: BLOCKED } }",
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.Replace(heldScenario, heldStepOne, step, 1)
			if body == heldScenario {
				t.Fatal("the replacement changed nothing")
			}
			if _, err := labspec.LoadScenario(writeFile(t, "case-01-allow-read.yaml", body)); !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
	body := strings.Replace(heldScenario, "2: { opens: none }", "2: { opens: none, result: { status: SUCCESS } }", 1)
	if _, err := labspec.LoadScenario(writeFile(t, "case-01-allow-read.yaml", body)); !errors.Is(err, labspec.ErrInvalid) {
		t.Errorf("a step that opens no trail stated a result: %v", err)
	}
}

// A held trail is closed once, so its result belongs to the step that resumes
// it; stated on the opening step as well, two steps would read one record.
func TestAResultOnAStepAnotherResumesIsRefused(t *testing.T) {
	body := strings.Replace(heldScenario, heldStepOne,
		"1: { verdict: REQUIRE_APPROVAL, trail: [ACTION_PROPOSED, POLICY_DECIDED, APPROVAL_REQUESTED, APPROVAL_DECIDED, ACTION_STARTED, ACTION_COMPLETED], result: { status: SUCCESS } }", 1)
	if body == heldScenario {
		t.Fatal("the replacement changed nothing")
	}
	scenario, trajectory := loadPair(t, body)
	if err := labspec.Validate(scenario, trajectory); !errors.Is(err, labspec.ErrInvalid) {
		t.Fatalf("Validate = %v, want ErrInvalid", err)
	}
	resumed := strings.Replace(heldScenario, "3: { resumes: 1, trail: [ACTION_PROPOSED, POLICY_DECIDED, APPROVAL_REQUESTED, APPROVAL_DECIDED, ACTION_STARTED, ACTION_COMPLETED] }",
		"3: { resumes: 1, trail: [ACTION_PROPOSED, POLICY_DECIDED, APPROVAL_REQUESTED, APPROVAL_DECIDED, ACTION_STARTED, ACTION_COMPLETED], result: { status: SUCCESS } }", 1)
	if resumed == heldScenario {
		t.Fatal("the replacement changed nothing")
	}
	scenario, trajectory = loadPair(t, resumed)
	if err := labspec.Validate(scenario, trajectory); err != nil {
		t.Errorf("a result on the resuming step was refused: %v", err)
	}
}
