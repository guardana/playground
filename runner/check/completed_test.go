package check_test

import (
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
)

func TestTrailClaimsHoldEveryCompletionToASuccessfulResult(t *testing.T) {
	for name, test := range map[string]struct {
		status string
		want   assertion.Outcome
		detail string
	}{
		"a successful result":   {"RESULT_STATUS_SUCCESS", assertion.Pass, ""},
		"a failed result":       {"RESULT_STATUS_FAILURE", assertion.Fail, "FAILURE"},
		"a result of no status": {"", assertion.Fail, "no status"},
	} {
		t.Run(name, func(t *testing.T) {
			events := enforced("ENFORCEMENT_MODE_ENFORCE", authorizedDigest)
			for i := range events {
				if events[i].Kind == evidence.KindActionCompleted {
					events[i].Result.Status = test.status
				}
			}
			got, found := runClaims(t, "enforce", events)["evidence/completed-succeeded"]
			if !found || got.Outcome != test.want || !strings.Contains(got.Detail, test.detail) {
				t.Errorf("got %v %s %q, want %s naming %q", found, got.Outcome, got.Detail, test.want, test.detail)
			}
		})
	}
}

func TestTrailClaimsReadNoCompletionWhenNothingCompleted(t *testing.T) {
	events := trail(decided{step: 1, requestID: "r1", verdict: "DENY", blocked: true})
	for i := range events {
		events[i].EnforcementMode = "ENFORCEMENT_MODE_ENFORCE"
	}
	if got, found := runClaims(t, "enforce", events)["evidence/completed-succeeded"]; found {
		t.Errorf("a run with no completion was graded on one: %+v", got)
	}
}
