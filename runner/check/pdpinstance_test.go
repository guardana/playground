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

const doubleIdentifier = "https://pdp-double:8443"

// consulted is one allowed trail whose decision names instance, empty for none.
func consulted(instance string) []evidence.Event {
	events := trail(decided{step: 1, requestID: "r1", verdict: "ALLOW", reasons: []string{"RULE_ALLOW"}})
	for i := range events {
		if events[i].Kind == evidence.KindPolicyDecided {
			events[i].Decision.PdpInstance = instance
		}
	}
	return events
}

func TestDecisionsGradeTheDecisionPointADecisionNames(t *testing.T) {
	for name, test := range map[string]struct {
		want, recorded string
		outcome        assertion.Outcome
		detail         string
	}{
		"the identifier, as recorded":             {doubleIdentifier, doubleIdentifier, assertion.Pass, ""},
		"none, and none consulted":                {labspec.PDPInstanceNone, "", assertion.Pass, ""},
		"none, and the decision names the double": {labspec.PDPInstanceNone, doubleIdentifier, assertion.Fail, doubleIdentifier},
		"the identifier, and none consulted":      {doubleIdentifier, "", assertion.Fail, "names decision point none"},
		"another identifier":                      {doubleIdentifier, "https://elsewhere:8443", assertion.Fail, "elsewhere"},
	} {
		t.Run(name, func(t *testing.T) {
			spec := scenario(map[int]labspec.DecisionExpectation{1: {Verdict: "ALLOW", PDPInstance: test.want}})
			checker := check.Decisions{Scenario: spec, Trajectory: sameTool(spec), EvidenceFile: evidenceFile}
			results, err := checker.Run(context.Background(), records(consulted(test.recorded), nil))
			if err != nil || len(results) != 1 {
				t.Fatalf("Run: %d results, %v", len(results), err)
			}
			if results[0].Outcome != test.outcome || !strings.Contains(results[0].Detail, test.detail) {
				t.Errorf("got %s %q, want %s naming %q", results[0].Outcome, results[0].Detail, test.outcome, test.detail)
			}
		})
	}
}

// A step that states no pdp_instance grades nothing about it, whatever the
// decision names.
func TestDecisionsLeaveAnUnstatedDecisionPointUngraded(t *testing.T) {
	spec := scenario(map[int]labspec.DecisionExpectation{1: {Verdict: "ALLOW"}})
	checker := check.Decisions{Scenario: spec, Trajectory: sameTool(spec), EvidenceFile: evidenceFile}
	results, err := checker.Run(context.Background(), records(consulted(doubleIdentifier), nil))
	if err != nil || len(results) != 1 || results[0].Outcome != assertion.Pass {
		t.Fatalf("got %+v, %v; want one pass", results, err)
	}
	if strings.Contains(results[0].Got, "pdp_instance") {
		t.Errorf("got %q reports a decision point nobody asked about", results[0].Got)
	}
}
