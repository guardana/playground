package check_test

import (
	"context"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

// A step that ran but whose record nobody read has established nothing: not a
// clean report, not an approving pin, and not an exit status worth grading.
func TestAStepWhoseRecordNobodyReadIsIndeterminate(t *testing.T) {
	const url = "http://victim-fs:8080/mcp"
	zero := 0
	want := labspec.VerifierExpectation{ExitCode: &zero, FindingsExclude: []string{"lab.rule.a"}, Gate: "pass"}
	spec := labspec.Scenario{
		Verifier: []labspec.VerifierStep{
			{Probe: &labspec.ProbeStep{Server: "victim-fs", WritePin: true}},
			{Probe: &labspec.ProbeStep{Server: "victim-fs", PinFrom: 1}},
		},
		Expect: labspec.Expect{Verifier: map[int]labspec.VerifierExpectation{1: {ExitCode: &zero}, 2: want}},
	}
	runs := []check.VerifierRun{
		{Step: 1, Probe: labspec.ProbeStep{Server: "victim-fs", WritePin: true}, URL: url, Ran: true, Pin: "pin-1.json"},
		{Step: 2, Probe: labspec.ProbeStep{Server: "victim-fs", PinFrom: 1}, URL: url, Ran: true, Report: "step-2.json"},
	}
	for name, subject := range map[string]assertion.Check{
		"verifier": check.Verifier{Scenario: spec, Runs: runs},
		"trace":    check.Trace{Analysis: runs[1], Want: want},
	} {
		results, err := subject.Run(context.Background(), assertion.Records{})
		if err != nil {
			t.Fatal(err)
		}
		wanted := map[string]int{"verifier": 6, "trace": 4}[name]
		if len(results) != wanted {
			t.Errorf("%s: %d results, want %d: %+v", name, len(results), wanted, results)
		}
		for _, result := range results {
			if result.Outcome != assertion.Indeterminate {
				t.Errorf("%s = %s beside a record nobody read (%q), want indeterminate", result.Check, result.Outcome, result.Got)
			}
		}
	}
}
