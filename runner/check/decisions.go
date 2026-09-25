package check

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
)

// Decisions grades every step the scenario names against the records of the
// trail that step opened, or the held trail it resumed.
//
// A step is joined to its trail through the record and never through the
// agent: trails pair with the steps that open one in the order they were
// opened, each proposing the tool its step calls.
type Decisions struct {
	Scenario labspec.Scenario
	// Trajectory names the tool each step calls, which the proposal paired
	// with the step has to name too.
	Trajectory labspec.Trajectory
	// EvidenceFile is named in every Source, because a red run is read by
	// someone who then opens that file at that line.
	EvidenceFile string
}

// ID names the check in a report.
func (Decisions) ID() string { return "decisions" }

// Run grades one result per step the scenario names.
func (d Decisions) Run(_ context.Context, records assertion.Records) ([]assertion.Result, error) {
	rows := DecisionRows(d.Scenario, d.Trajectory, records, d.EvidenceFile)
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
	// Wanted is what a named gap wants this step to be; printed, never graded.
	Wanted string
}

// DecisionRows grades every step the scenario names, in step order.
func DecisionRows(
	spec labspec.Scenario, trajectory labspec.Trajectory, records assertion.Records, evidenceFile string,
) []DecisionRow {
	paired := pairTrails(spec, trajectory, records.Evidence, records.RunID)
	steps := slices.Sorted(maps.Keys(spec.Expect.Decisions))
	rows := make([]DecisionRow, 0, len(steps))
	for _, step := range steps {
		row := gradeStep(spec, records, evidenceFile, paired, step)
		if spec.Gap != nil {
			if wanted, named := spec.Gap.Wanted[step]; named {
				row.Wanted = describeExpectation(wanted)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func gradeStep(spec labspec.Scenario, records assertion.Records, file string, paired pairing, step int) DecisionRow {
	want := spec.Expect.Decisions[step]
	row := DecisionRow{Step: step, Want: describeExpectation(want), Source: file}
	if want.Opens == labspec.OpensNone {
		row.Got, row.Outcome = "no trail of its own", assertion.Pass
		if !paired.balanced() {
			row.Outcome = assertion.Fail
			row.Detail = "whether this step wrote a trail cannot be told from the others: " + paired.counts()
		}
		return row
	}
	if want.Resumes != 0 && (want.Verdict != "" || len(want.ReasonCodesInclude) > 0 || len(want.ObligationsInclude) > 0 ||
		want.PDPInstance != "") {
		row.Outcome = assertion.Fail
		row.Detail = fmt.Sprintf("step %d resumes step %d and states a verdict, which only re-reads step %d's %s",
			step, want.Resumes, want.Resumes, evidence.KindPolicyDecided)
		return row
	}
	found, defect := paired.trailOf(step, want)
	if found.line > 0 {
		row.Source = fmt.Sprintf("%s:%d", file, found.line)
	}
	if defect != "" {
		row.Outcome, row.Detail = assertion.Fail, defect
		return row
	}
	tolerated := slices.Contains(spec.Tolerance.AllowIndeterminateForSteps, step)
	return gradeTrail(row, want, records, file, found.requestID, tolerated)
}

// gradeTrail grades what the step states against the one trail it is paired
// with: the kernel's verdict, then the block, then the whole sequence of kinds.
func gradeTrail(
	row DecisionRow, want labspec.DecisionExpectation, records assertion.Records, file, requestID string, tolerated bool,
) DecisionRow {
	events := records.Evidence
	var got []string
	fail := func(detail string) DecisionRow {
		row.Got, row.Outcome, row.Detail = strings.Join(got, "; "), assertion.Fail, detail
		return row
	}
	if want.Verdict != "" {
		decisions := eventsOn(events, records.RunID, requestID, evidence.KindPolicyDecided)
		if len(decisions) != 1 || events[decisions[0]].Decision == nil {
			return fail(decisionDefect(requestID, len(decisions)))
		}
		row.Source = fmt.Sprintf("%s:%d", file, decisions[0]+1)
		if row = grade(row, want, events[decisions[0]].Decision, tolerated); row.Outcome == assertion.Fail {
			return row
		}
		got = append(got, row.Got)
	}
	if want.Blocked != nil {
		detail, recorded := gradeBlock(*want.Blocked, events, records.RunID, requestID)
		got = append(got, recorded)
		if detail != "" {
			return fail(detail)
		}
	}
	if len(want.Trail) > 0 {
		kinds, err := kindsOf(events, requestID)
		got = append(got, "trail ["+strings.Join(kinds, " ")+"]")
		if detail := trailDefect(want.Trail, kinds, err); detail != "" {
			return fail(detail)
		}
	}
	row.Got = strings.Join(got, "; ")
	row.Outcome = assertion.Pass
	return row
}
