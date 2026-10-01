package check_test

import (
	"context"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

// The builders stamp every proposal with flow.v1.untrusted=false and
// flow.v1.max_read=PUBLIC.
func TestAStepsProposedTagsAreGraded(t *testing.T) {
	for name, tc := range map[string]struct {
		tags   []string
		want   assertion.Outcome
		detail string
	}{
		"every tag stated is there": {[]string{"flow.v1.untrusted=false", "flow.v1.max_read=PUBLIC"}, assertion.Pass, ""},
		"a tag the proposal lacks":  {[]string{"flow.v1.max_read=CONFIDENTIAL"}, assertion.Fail, "flow.v1.max_read=CONFIDENTIAL"},
	} {
		t.Run(name, func(t *testing.T) {
			spec := scenario(map[int]labspec.DecisionExpectation{1: {Verdict: "ALLOW", ProposedTagsInclude: tc.tags}})
			results, err := check.Decisions{Scenario: spec, Trajectory: sameTool(spec), EvidenceFile: "evidence.jsonl"}.
				Run(context.Background(), records(trail(decided{step: 1, requestID: "r1", verdict: "ALLOW"}), nil))
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != 1 || results[0].Outcome != tc.want || !strings.Contains(results[0].Detail, tc.detail) {
				t.Fatalf("results = %+v, want one %s naming %q", results, tc.want, tc.detail)
			}
		})
	}
}
