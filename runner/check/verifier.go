package check

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
)

// VerifierPrefix starts the name of every result the Verifier check reports.
const VerifierPrefix = "verifier/"

// VerifierRun is what the runner observed of one verifier step: whether the
// container ran, its exit status, and the files the step left.
type VerifierRun struct {
	Step  int
	Probe labspec.ProbeStep
	// URL is the endpoint the step probed.
	URL string
	// Ran reports that the container was started at all; Detail says why not.
	Ran      bool
	ExitCode int
	Detail   string
	// Report holds what the verifier printed on standard output; Pin is the
	// file a write_pin step approves the manifest in.
	Report string
	Pin    string
}

// Verifier grades each verifier step from the record it left: the pin a
// write_pin step wrote, or the JSON report any other step printed. The exit
// status is graded only beside a record, because a container that never
// reached the verifier exits non-zero too, and a 1 from docker is not a 1 from
// the verifier's policy gate.
type Verifier struct {
	Scenario labspec.Scenario
	Runs     []VerifierRun
}

// ID names the check in a report.
func (Verifier) ID() string { return "verifier" }

// Run grades every step the scenario states, in step order.
func (v Verifier) Run(_ context.Context, _ assertion.Records) ([]assertion.Result, error) {
	var results []assertion.Result
	for number := 1; number <= len(v.Scenario.Verifier); number++ {
		run, found := v.run(number)
		if !found {
			results = append(results, assertion.Result{
				Check: stepName(number, "ran"), Outcome: assertion.Indeterminate,
				Want: "the step run", Got: "the runner recorded nothing for it",
			})
			continue
		}
		results = append(results, v.grade(run, v.Scenario.Expect.Verifier[number])...)
	}
	return results, nil
}

func (v Verifier) run(number int) (VerifierRun, bool) {
	for _, run := range v.Runs {
		if run.Step == number {
			return run, true
		}
	}
	return VerifierRun{}, false
}

func (v Verifier) grade(run VerifierRun, want labspec.VerifierExpectation) []assertion.Result {
	if !run.Ran {
		return []assertion.Result{{
			Check: stepName(run.Step, "ran"), Outcome: assertion.Indeterminate,
			Want: "the verifier run", Got: "the verifier was not run", Detail: spoken(run.Detail),
		}}
	}
	if run.Probe.WritePin {
		_, err := readPin(run.Pin, run.URL)
		return []assertion.Result{
			recordResult(run, "pin", run.Pin, "a pin approving "+run.URL, err),
			exitResult(run, want, run.Pin, err),
		}
	}
	report, err := readReport(run.Report, run.URL)
	results := []assertion.Result{
		recordResult(run, "report", run.Report, "a JSON report about "+run.URL, err),
		exitResult(run, want, run.Report, err),
	}
	return append(results, gradeReport(run, want, report, err)...)
}

// recordResult fails a step whose record is missing or about something else:
// the verifier documents both files, and a missing record is never a pass.
func recordResult(run VerifierRun, what, source, want string, err error) assertion.Result {
	result := assertion.Result{Check: stepName(run.Step, what), Want: want, Source: source}
	if err != nil {
		result.Outcome, result.Got, result.Detail = assertion.Fail, "no such record", err.Error()
		return result
	}
	result.Outcome, result.Got = assertion.Pass, want
	return result
}

func exitResult(run VerifierRun, want labspec.VerifierExpectation, source string, unread error) assertion.Result {
	result := assertion.Result{
		Check: stepName(run.Step, "exit-code"), Want: fmt.Sprintf("exit %d", *want.ExitCode),
		Got: fmt.Sprintf("exit %d", run.ExitCode), Source: source,
	}
	switch {
	case unread != nil:
		result.Outcome = assertion.Indeterminate
		result.Detail = "the step left no record, so its exit status may be docker's and not the verifier's"
	case run.ExitCode != *want.ExitCode:
		result.Outcome = assertion.Fail
	default:
		result.Outcome = assertion.Pass
	}
	return result
}

// gradeReport grades what the report names. Beside a report that could not be
// read every expectation is indeterminate: it was never looked at.
func gradeReport(run VerifierRun, want labspec.VerifierExpectation, report verifierReport, unread error) []assertion.Result {
	var results []assertion.Result
	add := func(what, wanted string, grade func() (bool, string)) {
		result := assertion.Result{Check: stepName(run.Step, what), Want: wanted, Source: run.Report}
		if unread != nil {
			result.Outcome, result.Got = assertion.Indeterminate, "the report was not read"
			results = append(results, result)
			return
		}
		held, got := grade()
		result.Got, result.Outcome = got, assertion.Fail
		if held {
			result.Outcome = assertion.Pass
		}
		results = append(results, result)
	}
	for _, finding := range want.FindingsInclude {
		add("finding/"+finding.RuleID, describeFinding(finding), func() (bool, string) { return report.hasFinding(finding) })
	}
	for _, rule := range want.FindingsExclude {
		add("no-finding/"+rule, "the rule ran and reported nothing", func() (bool, string) { return report.concludedClean(rule) })
	}
	for _, rule := range want.UnverifiedInclude {
		add("unverified/"+rule, "the rule ran and could not reach a verdict", func() (bool, string) {
			if slices.Contains(ruleIDs(report.Unverified), rule) {
				return true, "reported unverified"
			}
			return false, "not among the unverified results"
		})
	}
	return results
}

func stepName(number int, what string) string {
	return fmt.Sprintf("%sstep-%d/%s", VerifierPrefix, number, what)
}

func describeFinding(want labspec.FindingExpectation) string {
	parts := []string{want.RuleID}
	if want.Severity != "" {
		parts = append(parts, want.Severity)
	}
	if want.SummaryContains != "" {
		parts = append(parts, fmt.Sprintf("summary containing %q", want.SummaryContains))
	}
	return strings.Join(parts, ", ")
}
