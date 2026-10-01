package check

import (
	"fmt"
	"strings"

	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
)

// gradeResult grades the one closing record on a request against the result
// the step states. It returns the defect, empty when there is none, and what
// was recorded.
func gradeResult(want labspec.ResultExpectation, events []evidence.Event, run, requestID string) (string, string) {
	closings := append(eventsOn(events, run, requestID, evidence.KindActionCompleted),
		eventsOn(events, run, requestID, evidence.KindActionFailed)...)
	if len(closings) != 1 {
		return fmt.Sprintf("request %q carries %d closing records (%s or %s), want one", requestID, len(closings),
			evidence.KindActionCompleted, evidence.KindActionFailed), "no one closing record"
	}
	closing := events[closings[0]]
	if closing.Result == nil {
		return fmt.Sprintf("the %s on request %q carries no result", closing.Kind, requestID), "a closing record with no result"
	}
	got := strings.TrimPrefix(closing.Result.Status, resultStatusPrefix)
	if closing.Result.Status != resultStatusPrefix+want.Status {
		return fmt.Sprintf("the closing result is %s and the scenario expects %s", got, want.Status), "result " + got
	}
	return "", "result " + got
}
