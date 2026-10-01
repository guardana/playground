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

// closedWith is one allowed request whose closing record, the fourth line,
// carries a result preview and is of the given kind.
func closedWith(kind evidence.Kind, preview string) []evidence.Event {
	events := trail(decided{step: 1, requestID: "r1", verdict: "ALLOW"})
	events[3].Kind = kind
	events[3].Result = &evidence.ActionResult{RequestID: "r1", RedactedResultPreview: preview}
	return events
}

func captureOf(t *testing.T, captured bool, events []evidence.Event) assertion.Result {
	t.Helper()
	checker := check.Evidence{
		Scenario:     evidenceScenario(labspec.EvidenceExpectation{ContentCaptured: captured}),
		EvidenceFile: evidenceFile,
	}
	results, err := checker.Run(context.Background(), records(events, nil))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return result(t, results, "evidence/content-captured")
}

// Captured content is argument text and result text alike, so a result preview
// breaks the privacy default as surely as an argument preview does.
func TestContentCaptureReadsTheResultPreviewOfAClosingRecord(t *testing.T) {
	canary := strings.Join([]string{"CANARY", "0002", "returned"}, "-")
	tests := []struct {
		name       string
		captured   bool
		recorded   []evidence.Event
		want       assertion.Outcome
		wantSource string
	}{
		{"a completed result preview where none is expected", false,
			closedWith(evidence.KindActionCompleted, canary), assertion.Fail, "reports/run-1/evidence.jsonl:4"},
		{"a failed result preview where none is expected", false,
			closedWith(evidence.KindActionFailed, canary), assertion.Fail, "reports/run-1/evidence.jsonl:4"},
		{"a result preview alone where one is expected", true,
			closedWith(evidence.KindActionCompleted, canary), assertion.Pass, "reports/run-1/evidence.jsonl"},
		{"an argument preview where none is expected", false,
			trail(decided{step: 1, requestID: "r1", verdict: "ALLOW", preview: canary}), assertion.Fail,
			"reports/run-1/evidence.jsonl:1"},
		{"no preview of either kind where one is expected", true,
			closedWith(evidence.KindActionCompleted, ""), assertion.Fail, "reports/run-1/evidence.jsonl"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := captureOf(t, test.captured, test.recorded)
			if got.Outcome != test.want || got.Source != test.wantSource {
				t.Errorf("outcome %s at %q, want %s at %q (%+v)", got.Outcome, got.Source, test.want, test.wantSource, got)
			}
		})
	}
}

func TestContentCaptureNamesBothPreviewsItReads(t *testing.T) {
	canary := strings.Join([]string{"CANARY", "0003", "returned"}, "-")
	got := captureOf(t, false, closedWith(evidence.KindActionCompleted, canary))
	for _, field := range []string{"arguments.redactedPreview", "result.redactedResultPreview"} {
		if !strings.Contains(got.Want, field) {
			t.Errorf("want %q does not name %s", got.Want, field)
		}
	}
	if !strings.Contains(got.Got, "1 of 1 closing records carry a result preview") {
		t.Errorf("got %q does not count the result preview", got.Got)
	}
	if !strings.Contains(got.Detail, "line 4") {
		t.Errorf("detail %q does not name the line that carries content", got.Detail)
	}
}
