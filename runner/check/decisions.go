package check

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
)

// Decisions grades every step the scenario names against the verdict the trail
// recorded for it.
//
// A step is joined to its decision through the record and never through the
// agent: the proposed envelope carries the step number in context.stepId, and
// the POLICY_DECIDED event on the same requestId carries the verdict.
type Decisions struct {
	Scenario labspec.Scenario
	// EvidenceFile is named in every Source, because a red run is read by
	// someone who then opens that file at that line.
	EvidenceFile string
}

// ID names the check in a report.
func (Decisions) ID() string { return "decisions" }

// Run grades one result per step the scenario names.
func (d Decisions) Run(_ context.Context, records assertion.Records) ([]assertion.Result, error) {
	rows := DecisionRows(d.Scenario, records, d.EvidenceFile)
	results := make([]assertion.Result, 0, len(rows))
	for _, row := range rows {
		results = append(results, assertion.Result{
			Check:   fmt.Sprintf("decisions/step-%d", row.Step),
			Outcome: row.Outcome,
			Want:    row.Want,
			Got:     row.Got,
			Source:  row.Source,
			Detail:  row.Detail,
		})
	}
	return results, nil
}

// DecisionRow is one step's grade and everything the report table shows about
// it. The check and the written report build it from the same function, so the
// table a person reads and the outcome the build gates on cannot disagree.
type DecisionRow struct {
	Step        int
	Want        string
	Got         string
	ReasonCodes []string
	Obligations []string
	Source      string
	Outcome     assertion.Outcome
	Detail      string
}

// DecisionRows grades every step the scenario names, in step order.
func DecisionRows(spec labspec.Scenario, records assertion.Records, evidenceFile string) []DecisionRow {
	steps := slices.Sorted(maps.Keys(spec.Expect.Decisions))
	rows := make([]DecisionRow, 0, len(steps))
	for _, step := range steps {
		rows = append(rows, gradeStep(spec, records, evidenceFile, step))
	}
	return rows
}

// gradeStep finds the one trail that claims the step and grades it. Anything
// other than exactly one proposal and exactly one decision is a failure: the
// record that should exist either does not, or says two things at once.
func gradeStep(spec labspec.Scenario, records assertion.Records, file string, step int) DecisionRow {
	want := spec.Expect.Decisions[step]
	events := records.Evidence
	row := DecisionRow{Step: step, Want: describeExpectation(want), Source: file}

	proposals := proposalsForStep(events, records.RunID, step)
	if len(proposals) != 1 {
		row.Outcome = assertion.Fail
		row.Detail = proposalDefect(events, proposals, step)
		return row
	}
	requestID := events[proposals[0]].RequestID
	decisions := decisionsForRequest(events, records.RunID, requestID)
	if len(decisions) != 1 {
		row.Outcome = assertion.Fail
		row.Source = fmt.Sprintf("%s:%d", file, proposals[0]+1)
		row.Detail = decisionDefect(requestID, len(decisions))
		return row
	}
	// One event per line and no blank line, so the index is the line number.
	row.Source = fmt.Sprintf("%s:%d", file, decisions[0]+1)
	return grade(row, want, events[decisions[0]].Decision,
		slices.Contains(spec.Tolerance.AllowIndeterminateForSteps, step))
}

func decisionDefect(requestID string, found int) string {
	if found == 0 {
		return fmt.Sprintf("no %s event on request %q, so the step was proposed and never decided",
			evidence.KindPolicyDecided, requestID)
	}
	return fmt.Sprintf("request %q carries %d %s events, so no one verdict is its own",
		requestID, found, evidence.KindPolicyDecided)
}

func proposalDefect(events []evidence.Event, proposals []int, step int) string {
	if len(proposals) == 0 {
		return fmt.Sprintf("no proposed envelope carries context.stepId %q", strconv.Itoa(step))
	}
	ids := make([]string, 0, len(proposals))
	for _, index := range proposals {
		ids = append(ids, fmt.Sprintf("%q at line %d", events[index].RequestID, index+1))
	}
	return fmt.Sprintf("step %d is claimed by %d proposed envelopes (%s), so no one decision is its own",
		step, len(proposals), strings.Join(ids, ", "))
}

// grade compares the recorded decision with the expectation. The verdict is
// compared first: reason codes attached to the wrong verdict grade nothing.
func grade(row DecisionRow, want labspec.DecisionExpectation, decision *evidence.Decision, tolerated bool) DecisionRow {
	got := decision.ShortVerdict()
	row.ReasonCodes = decision.ReasonCodes
	row.Obligations = obligationTypes(decision.Obligations)
	row.Got = describeDecision(got, row.ReasonCodes, row.Obligations)

	switch {
	case got == want.Verdict:
	case got == "INDETERMINATE" && tolerated:
		row.Outcome = assertion.Pass
		row.Detail = fmt.Sprintf(
			"the verdict is INDETERMINATE where %s was expected, and the scenario's tolerance names this step",
			want.Verdict)
		return row
	default:
		row.Outcome = assertion.Fail
		row.Detail = fmt.Sprintf("the recorded verdict is %s and the scenario expects %s", verdictName(got), want.Verdict)
		return row
	}
	if missing := absent(want.ReasonCodesInclude, row.ReasonCodes); len(missing) > 0 {
		row.Outcome = assertion.Fail
		row.Detail = "the decision does not carry reason code " + strings.Join(missing, ", ")
		return row
	}
	if missing := absent(want.ObligationsInclude, row.Obligations); len(missing) > 0 {
		row.Outcome = assertion.Fail
		row.Detail = "the decision does not carry obligation " + strings.Join(missing, ", ")
		return row
	}
	row.Outcome = assertion.Pass
	return row
}

// proposalsForStep finds the envelopes claiming the step, by line. An envelope
// stamped with another run claims a step of that run, and is not read here: a
// trail left by an earlier run would otherwise grade this one.
func proposalsForStep(events []evidence.Event, runID string, step int) []int {
	wanted := strconv.Itoa(step)
	var found []int
	for i, event := range events {
		if event.Kind != evidence.KindActionProposed || event.Proposed == nil {
			continue
		}
		if !writtenForRun(runIDs(event), runID) {
			continue
		}
		if event.Proposed.Context != nil && event.Proposed.Context.StepID == wanted {
			found = append(found, i)
		}
	}
	return found
}

func decisionsForRequest(events []evidence.Event, runID, requestID string) []int {
	var found []int
	for i, event := range events {
		if event.Kind != evidence.KindPolicyDecided || event.RequestID != requestID || event.Decision == nil {
			continue
		}
		if !writtenForRun(runIDs(event), runID) {
			continue
		}
		found = append(found, i)
	}
	return found
}

func obligationTypes(obligations []evidence.Obligation) []string {
	types := make([]string, 0, len(obligations))
	for _, obligation := range obligations {
		types = append(types, obligation.Type)
	}
	return types
}

// absent returns the wanted values the recorded list does not hold. Inclusion
// rather than equality: a decision may carry more than the scenario names.
func absent(wanted, recorded []string) []string {
	var missing []string
	for _, value := range wanted {
		if !slices.Contains(recorded, value) {
			missing = append(missing, value)
		}
	}
	return missing
}

func describeExpectation(want labspec.DecisionExpectation) string {
	return describeDecision(want.Verdict, want.ReasonCodesInclude, want.ObligationsInclude)
}

func describeDecision(verdict string, reasons, obligations []string) string {
	parts := []string{verdictName(verdict)}
	if len(reasons) > 0 {
		parts = append(parts, "reason codes ["+strings.Join(reasons, " ")+"]")
	}
	if len(obligations) > 0 {
		parts = append(parts, "obligations ["+strings.Join(obligations, " ")+"]")
	}
	return strings.Join(parts, ", ")
}

// verdictName names an empty verdict rather than printing nothing, because
// nothing decided has to read differently from a verdict this reader failed to
// print.
func verdictName(verdict string) string {
	if verdict == "" {
		return "no verdict"
	}
	return verdict
}
