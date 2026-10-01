package check_test

import (
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
)

// A stated trail is the whole trail, not a prefix of it: a block followed by a
// start is an action that ran after it was refused.
func TestARecordedTrailLongerThanTheStatedOneFails(t *testing.T) {
	stated := []string{"ACTION_PROPOSED", "POLICY_DECIDED", "ACTION_BLOCKED"}
	blocked := func() *plane {
		p := &plane{}
		return p.add("req-denied", evidence.KindActionProposed, nil).
			add("req-denied", evidence.KindPolicyDecided, verdict("DENY", "RULE_DENY")).
			add("req-denied", evidence.KindActionBlocked, nil)
	}
	spec := scenario(map[int]labspec.DecisionExpectation{1: {Verdict: "DENY", Trail: stated}})

	if result := grade(t, spec, blocked().events)["decisions/step-1"]; result.Outcome != assertion.Pass {
		t.Fatalf("the stated trail itself was %s: %s", result.Outcome, result.Detail)
	}

	longer := blocked().add("req-denied", evidence.KindActionStarted, nil)
	result := grade(t, spec, longer.events)["decisions/step-1"]
	if result.Outcome != assertion.Fail {
		t.Fatalf("a trail one kind longer than stated was %s: %+v", result.Outcome, result)
	}
	if !strings.Contains(result.Detail, "ACTION_BLOCKED ACTION_STARTED") {
		t.Errorf("the defect does not show the extra kind: %s", result.Detail)
	}
}
