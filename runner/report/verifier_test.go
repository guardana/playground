package report_test

import (
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/runner/report"
)

// A verifier run grades no decision by design; saying that no step was graded
// would send a reader looking for a trail that was never meant to exist.
func TestMarkdownSaysAVerifierRunHasNoDecisionToGrade(t *testing.T) {
	graded := assertion.Report{Scenario: "verify-01", Results: []assertion.Result{
		{Check: "verifier/step-1/exit-code", Outcome: assertion.Pass},
	}}
	var out strings.Builder
	if err := report.WriteMarkdown(&out, graded, nil, report.Provenance{}); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	if strings.Contains(out.String(), "No step was graded") || !strings.Contains(out.String(), "verifier scenario") {
		t.Errorf("a verifier report does not say why it grades no decision:\n%s", out.String())
	}
}
