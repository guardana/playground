package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
)

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

// gradeBlock reads the one ACTION_BLOCKED on the request: the block a mode or
// the plane made, which POLICY_DECIDED does not carry. It returns the defect,
// empty when the block is as stated, and what was recorded.
func gradeBlock(want labspec.BlockExpectation, events []evidence.Event, runID, requestID string) (string, string) {
	blocks := eventsOn(events, runID, requestID, evidence.KindActionBlocked)
	if len(blocks) != 1 {
		return fmt.Sprintf("request %q carries %d %s events, want one", requestID, len(blocks), evidence.KindActionBlocked),
			"no one block"
	}
	decision := events[blocks[0]].Decision
	if decision == nil {
		return fmt.Sprintf("the %s on request %q carries no decision", evidence.KindActionBlocked, requestID), "a block with no decision"
	}
	recorded := "blocked " + describeDecision(decision.ShortVerdict(), decision.ReasonCodes, nil)
	if got := decision.ShortVerdict(); got != want.Verdict {
		return fmt.Sprintf("the block's verdict is %s and the scenario expects %s", verdictName(got), want.Verdict), recorded
	}
	if missing := absent(want.ReasonCodesInclude, decision.ReasonCodes); len(missing) > 0 {
		return "the block does not carry reason code " + strings.Join(missing, ", "), recorded
	}
	return "", recorded
}

func trailDefect(want, got []string, orderErr error) string {
	if orderErr != nil {
		return "the trail's links give no one order: " + orderErr.Error()
	}
	if !slices.Equal(want, got) {
		return fmt.Sprintf("the trail holds [%s] and the scenario expects [%s]",
			strings.Join(got, " "), strings.Join(want, " "))
	}
	return ""
}

func decisionDefect(requestID string, found int) string {
	if found == 0 {
		return fmt.Sprintf("no %s event on request %q, so the step was proposed and never decided",
			evidence.KindPolicyDecided, requestID)
	}
	return fmt.Sprintf("request %q carries %d %s events, so no one verdict is its own",
		requestID, found, evidence.KindPolicyDecided)
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
	if want.Opens != "" {
		return "opens " + want.Opens
	}
	var parts []string
	if want.Resumes != 0 {
		parts = append(parts, fmt.Sprintf("resumes step %d", want.Resumes))
	}
	if want.Verdict != "" {
		parts = append(parts, describeDecision(want.Verdict, want.ReasonCodesInclude, want.ObligationsInclude))
	}
	if want.Blocked != nil {
		parts = append(parts, "blocked "+describeDecision(want.Blocked.Verdict, want.Blocked.ReasonCodesInclude, nil))
	}
	if len(want.Trail) > 0 {
		parts = append(parts, "trail ["+strings.Join(want.Trail, " ")+"]")
	}
	return strings.Join(parts, "; ")
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
