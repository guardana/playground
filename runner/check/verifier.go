package check

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/internal/record"
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
	// Record is the report as the runner read it and RecordErr why it could
	// not be read; both are nil when nobody read it.
	Record    *record.Run
	RecordErr error
	// Approved is what a write_pin step's pin approves and ApprovedErr why it
	// could not be read; both are nil when nobody read it.
	Approved    *record.Approval
	ApprovedErr error
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
			Want: "the verifier run", Got: "the verifier was not run", Detail: Spoken(run.Detail),
		}}
	}
	if run.Probe.WritePin {
		read := run.Approved != nil || run.ApprovedErr != nil
		err := approvalErr(run)
		return []assertion.Result{
			recordResult(run, "pin", run.Pin, "a pin approving "+run.URL, read, err),
			exitResult(run, want, run.Pin, read, err),
		}
	}
	read := run.Record != nil || run.RecordErr != nil
	ran, err := reportOf(run)
	results := []assertion.Result{
		recordResult(run, "report", run.Report, "a JSON report about "+run.URL, read, err),
		exitResult(run, want, run.Report, read, err),
	}
	return append(results, gradeReport(run, want, ran, err)...)
}

// approvalErr says why the pin a step wrote does not approve the server it
// probed, if it does not.
func approvalErr(run VerifierRun) error {
	approved := run.Approved
	switch {
	case run.ApprovedErr != nil:
		return run.ApprovedErr
	case approved == nil:
		return errors.New("the pin was not read")
	case approved.Server != run.URL:
		return fmt.Errorf("the pin approves %q, not the probed %s", approved.Server, run.URL)
	case approved.Tools == 0:
		return errors.New("the pin approves no tool")
	}
	return nil
}

// reportOf is the step's report, or why there is none to grade. A report
// nobody read is never an empty one.
func reportOf(run VerifierRun) (record.Run, error) {
	switch {
	case run.RecordErr != nil:
		return record.Run{}, run.RecordErr
	case run.Record == nil:
		return record.Run{}, errors.New("the report was not read")
	}
	return *run.Record, nil
}

// recordResult fails a step whose record is missing or about something else:
// the verifier documents both files, and a missing record is never a pass. A
// record nobody read has established nothing either way.
func recordResult(run VerifierRun, what, source, want string, read bool, err error) assertion.Result {
	result := assertion.Result{Check: stepName(run.Step, what), Want: want, Source: source}
	switch {
	case !read:
		result.Outcome, result.Got, result.Detail = assertion.Indeterminate, "the record was not read", err.Error()
		return result
	case err != nil:
		result.Outcome, result.Got, result.Detail = assertion.Fail, "no such record", err.Error()
		return result
	}
	result.Outcome, result.Got = assertion.Pass, want
	return result
}

func exitResult(run VerifierRun, want labspec.VerifierExpectation, source string, read bool, unread error) assertion.Result {
	result := assertion.Result{
		Check: stepName(run.Step, "exit-code"), Want: fmt.Sprintf("exit %d", *want.ExitCode),
		Got: fmt.Sprintf("exit %d", run.ExitCode), Source: source,
	}
	switch {
	case !read:
		result.Outcome = assertion.Indeterminate
		result.Detail = "the step's record was not read, so its exit status may be docker's and not the verifier's"
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

// gradeReport grades what the step's record names. Beside a report that could
// not be read every expectation is indeterminate: it was never looked at.
func gradeReport(run VerifierRun, want labspec.VerifierExpectation, ran record.Run, unread error) []assertion.Result {
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
		add("finding/"+finding.RuleID, describeFinding(finding), func() (bool, string) { return hasFinding(ran, finding) })
	}
	for _, rule := range want.FindingsExclude {
		add("no-finding/"+rule, "the rule ran and reported nothing", func() (bool, string) { return concludedClean(ran, rule) })
	}
	gradeVerdict(want, ran, add)
	gradeScan(want, ran, run.URL+"/", add)
	for _, rule := range want.UnverifiedInclude {
		add("unverified/"+rule, "the rule ran and could not reach a verdict", func() (bool, string) { return hasUnverified(ran, rule) })
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
