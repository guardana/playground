package check

import (
	"fmt"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
)

// resultStatusPrefix is how the wire spells a result status; a scenario names
// one without it.
const resultStatusPrefix = "RESULT_STATUS_"

// completedResult holds every ACTION_COMPLETED to a successful result: the
// enforcer's contract writes that kind only when the authorized bytes ran and
// the result succeeded, so a completion carrying any other status contradicts
// its own kind. Nothing completed is nothing to grade.
func (c TrailClaims) completedResult(events []evidence.Event) (assertion.Result, bool) {
	result := assertion.Result{
		Check:  "evidence/completed-succeeded",
		Want:   "every " + string(evidence.KindActionCompleted) + " carries " + resultStatusPrefix + "SUCCESS",
		Source: c.EvidenceFile,
	}
	completed, defects, firstLine := 0, []string{}, 0
	for i, event := range events {
		if event.Kind != evidence.KindActionCompleted {
			continue
		}
		completed++
		status := ""
		if event.Result != nil {
			status = event.Result.Status
		}
		if status == resultStatusPrefix+"SUCCESS" {
			continue
		}
		if firstLine == 0 {
			firstLine = i + 1
		}
		defects = append(defects, fmt.Sprintf("line %d on request %q: %s", i+1, event.RequestID, statusName(status)))
	}
	if completed == 0 {
		return result, false
	}
	result.Got = fmt.Sprintf("%d of %d succeeded", completed-len(defects), completed)
	if len(defects) > 0 {
		result.Outcome = assertion.Fail
		result.Source = fmt.Sprintf("%s:%d", c.EvidenceFile, firstLine)
		result.Detail = strings.Join(first5(defects), "; ")
		return result, true
	}
	result.Outcome = assertion.Pass
	return result, true
}

func statusName(status string) string {
	if status == "" {
		return "no status"
	}
	return strings.TrimPrefix(status, resultStatusPrefix)
}
