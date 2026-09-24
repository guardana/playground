package check_test

import (
	"context"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

// "No envelope carries a preview" is true of a trail that holds no envelope,
// so a trail without ACTION_PROPOSED says nothing about the privacy default.
func TestContentCaptureIsNotPassedOnATrailWithNoEnvelope(t *testing.T) {
	var withoutProposals []evidence.Event
	for _, event := range trail(decided{step: 1, requestID: "r1", verdict: "ALLOW"}) {
		if event.Kind != evidence.KindActionProposed {
			withoutProposals = append(withoutProposals, event)
		}
	}
	if len(withoutProposals) == 0 {
		t.Fatal("the trail held nothing but proposals")
	}
	checker := check.Evidence{
		Scenario:     evidenceScenario(labspec.EvidenceExpectation{ContentCaptured: false}),
		EvidenceFile: evidenceFile,
	}
	results, err := checker.Run(context.Background(), records(withoutProposals, nil))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := result(t, results, "evidence/content-captured"); got.Outcome == assertion.Pass {
		t.Errorf("content-captured passed on a trail with no envelope: %+v", got)
	}
}
