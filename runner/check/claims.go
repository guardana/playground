package check

import (
	"context"
	"fmt"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
)

// TrailClaims grades what the enforcer's trail claims apart from any one step:
// the mode every event was enforced under, that what ran is what was decided,
// and that every completion succeeded.
type TrailClaims struct {
	Scenario     labspec.Scenario
	EvidenceFile string
}

// ID names the check in a report.
func (TrailClaims) ID() string { return "trail-claims" }

// Run grades the mode always, and the executed digests when anything closed
// or started carrying one: a run where every call was blocked has no digest to
// compare, and the effects check reads what its victims served.
func (c TrailClaims) Run(_ context.Context, records assertion.Records) ([]assertion.Result, error) {
	results := []assertion.Result{c.modeResult(records.Evidence)}
	if executed, found := c.executedResult(records.Evidence); found {
		results = append(results, executed)
	}
	if completed, found := c.completedResult(records.Evidence); found {
		results = append(results, completed)
	}
	return results, nil
}

// modeResult holds every event to the scenario's enforcement_mode. The event's
// own field is the mode the enforcement point applied; the decision's is the
// kernel's and stays ENFORCE whatever the point did with it.
func (c TrailClaims) modeResult(events []evidence.Event) assertion.Result {
	want := "ENFORCEMENT_MODE_" + strings.ToUpper(c.Scenario.EnforcementMode)
	result := assertion.Result{
		Check:  "evidence/enforcement-mode",
		Want:   "every event carries enforcementMode " + want,
		Source: c.EvidenceFile,
	}
	if len(events) == 0 {
		result.Outcome, result.Got = assertion.Indeterminate, "no events to read a mode from"
		return result
	}
	var other []string
	firstLine := 0
	for i, event := range events {
		if event.EnforcementMode == want {
			continue
		}
		if firstLine == 0 {
			firstLine = i + 1
		}
		other = append(other, fmt.Sprintf("line %d %s", i+1, spoken(event.EnforcementMode)))
	}
	result.Got = fmt.Sprintf("%d of %d events carry it", len(events)-len(other), len(events))
	if len(other) > 0 {
		result.Outcome = assertion.Fail
		result.Source = fmt.Sprintf("%s:%d", c.EvidenceFile, firstLine)
		result.Detail = "the run was enforced under another mode: " + strings.Join(first5(other), ", ")
		return result
	}
	result.Outcome = assertion.Pass
	return result
}

// executedResult compares each ACTION_COMPLETED, and each ACTION_STARTED that
// carries a result, with the one POLICY_DECIDED on its request. A completion
// with no executed digest is a comparison that did not run, which the contract
// says is not a pass.
func (c TrailClaims) executedResult(events []evidence.Event) (assertion.Result, bool) {
	result := assertion.Result{
		Check: "evidence/executed-digest",
		Want: "every " + string(evidence.KindActionCompleted) +
			" carries executedActionDigest equal to its request's POLICY_DECIDED actionDigest",
		Source: c.EvidenceFile,
	}
	compared, defects := 0, []string{}
	firstLine := 0
	for i, event := range events {
		completed := event.Kind == evidence.KindActionCompleted
		if !completed && (event.Kind != evidence.KindActionStarted || event.Result == nil) {
			continue
		}
		compared++
		if defect := executedDefect(events, event); defect != "" {
			if firstLine == 0 {
				firstLine = i + 1
			}
			defects = append(defects, fmt.Sprintf("line %d: %s", i+1, defect))
		}
	}
	if compared == 0 {
		return result, false
	}
	result.Got = fmt.Sprintf("%d of %d equal", compared-len(defects), compared)
	if len(defects) > 0 {
		result.Outcome = assertion.Fail
		result.Source = fmt.Sprintf("%s:%d", c.EvidenceFile, firstLine)
		result.Detail = strings.Join(first5(defects), "; ")
		return result, true
	}
	result.Outcome = assertion.Pass
	return result, true
}

func executedDefect(events []evidence.Event, closing evidence.Event) string {
	executed := ""
	if closing.Result != nil {
		executed = closing.Result.ExecutedActionDigest
	}
	if executed == "" {
		return fmt.Sprintf("the %s on request %q carries no executedActionDigest, so the comparison did not run",
			closing.Kind, closing.RequestID)
	}
	var decided []evidence.Event
	for _, event := range events {
		if event.Kind == evidence.KindPolicyDecided && event.RequestID == closing.RequestID && event.Decision != nil {
			decided = append(decided, event)
		}
	}
	if len(decided) != 1 {
		return fmt.Sprintf("request %q carries %d %s events with a decision, want one to read what it authorized",
			closing.RequestID, len(decided), evidence.KindPolicyDecided)
	}
	if authorized := decided[0].Decision.ActionDigest; executed != authorized {
		return fmt.Sprintf("request %q executed %s and its decision authorized %s",
			closing.RequestID, executed, spoken(authorized))
	}
	return ""
}

// first5 keeps a detail readable when a whole trail disagrees.
func first5(items []string) []string {
	if len(items) > 5 {
		return append(items[:5:5], fmt.Sprintf("and %d more", len(items)-5))
	}
	return items
}
