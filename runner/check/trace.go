package check

import (
	"context"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
)

// Trace grades the verifier's analysis of the agent's own trace against the
// scenario's contract, from the report the verifier wrote. The report has to be
// about this run's trace; its exit code and findings are then read as a
// verifier step's are.
type Trace struct {
	Analysis VerifierRun
	Want     labspec.VerifierExpectation
}

// ID names the check in a report.
func (Trace) ID() string { return "trace" }

// Run grades the analysis under the trace/ prefix.
func (t Trace) Run(_ context.Context, _ assertion.Records) ([]assertion.Result, error) {
	results := Verifier{}.grade(t.Analysis, t.Want)
	prefix := stepName(t.Analysis.Step, "")
	for i := range results {
		results[i].Check = "trace/" + strings.TrimPrefix(results[i].Check, prefix)
	}
	return results, nil
}
