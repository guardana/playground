package check

import (
	"fmt"
	"strings"

	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
)

// recordGrader grades one thing a step states from one record of its trail and
// returns the defect, empty when there is none, and what was recorded.
type recordGrader func() (string, string)

// recordGraders lists what a step states beyond its decision, in the order a
// report reads them: the block, the sequence of kinds, the closing result and
// the tags of the proposal.
func recordGraders(want labspec.DecisionExpectation, events []evidence.Event, run, requestID string) []recordGrader {
	var graders []recordGrader
	if want.Blocked != nil {
		graders = append(graders, func() (string, string) { return gradeBlock(*want.Blocked, events, run, requestID) })
	}
	if len(want.Trail) > 0 {
		graders = append(graders, func() (string, string) {
			kinds, err := kindsOf(events, requestID)
			return trailDefect(want.Trail, kinds, err), "trail [" + strings.Join(kinds, " ") + "]"
		})
	}
	if want.Result != nil {
		graders = append(graders, func() (string, string) { return gradeResult(*want.Result, events, run, requestID) })
	}
	if len(want.ProposedTagsInclude) > 0 {
		graders = append(graders, func() (string, string) { return gradeTags(want.ProposedTagsInclude, events, run, requestID) })
	}
	return graders
}

// gradeTags reads the run-context tags on the trail's one ACTION_PROPOSED, where
// the gateway records the flow state its decision used.
func gradeTags(want []string, events []evidence.Event, run, requestID string) (string, string) {
	proposals := eventsOn(events, run, requestID, evidence.KindActionProposed)
	if len(proposals) != 1 {
		return fmt.Sprintf("request %q carries %d %s events, want one to read its tags", requestID, len(proposals),
			evidence.KindActionProposed), "no one proposal"
	}
	var tags []string
	if proposed := events[proposals[0]].Proposed; proposed != nil && proposed.Context != nil {
		tags = proposed.Context.Tags
	}
	recorded := "proposed tags [" + strings.Join(tags, " ") + "]"
	if missing := absent(want, tags); len(missing) > 0 {
		return "the proposal does not carry tag " + strings.Join(missing, ", "), recorded
	}
	return "", recorded
}
