// Package check holds the assertions a scenario run is graded by.
//
// Each one reads records the run left behind — which services came up, what the
// agent could reach, the evidence trail, the victims' journals — and reports
// what it established. None of them asks a running service how it is, and none
// of them reads the agent's own account of its work.
//
// The rule they share: an assertion over an empty set of records is
// indeterminate and never satisfied. "No envelope carries a preview" and "every
// decision carries a digest" are both true of a trail with nothing in it, and a
// check that reported those as passes would let a run that recorded nothing be
// read as green.
package check

import (
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/journal"
)

// spoken renders what a service or a probe said, naming silence rather than
// printing nothing, so a report never shows an empty reason as a reason.
func spoken(detail string) string {
	if detail == "" {
		return "nothing"
	}
	return detail
}

// planeRun is the run the trail records. The enforcer mints a run's id itself
// and, with no authenticator, keeps one run per process, so the id says nothing
// about which lab run wrote the trail: the run directory the runner made does.
// The first proposal naming a run names it; a record naming any other is from
// another process, is never read as this run's, and fails evidence/run-id.
func planeRun(events []evidence.Event) string {
	for _, event := range events {
		if event.Kind == evidence.KindActionProposed && event.RequestID != "" && event.RunID != "" {
			return event.RunID
		}
	}
	return ""
}

// writtenForRun reports whether a record may be read as the trail's run's. A
// record naming no run belongs to a call the enforcer gave none, such as one
// it refused before it computed a run; evidence/run-id asserts that it is.
func writtenForRun(event evidence.Event, run string) bool {
	return event.RunID == "" || event.RunID == run
}

// servedInRun keeps the journal entries this run's calls were recorded in, and
// counts the ones left out. A journal is appended to and outlives one run.
func servedInRun(entries []journal.Entry, runID string) (mine []journal.Entry, elsewhere int) {
	for _, entry := range entries {
		if entry.RunID != "" && entry.RunID != runID {
			elsewhere++
			continue
		}
		mine = append(mine, entry)
	}
	return mine, elsewhere
}
