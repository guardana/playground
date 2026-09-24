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

// runIDs are the runs an event names: the enforcement plane stamps one on the
// event and one on the proposed envelope's context, and either of them naming
// another run makes the event a record of something else.
func runIDs(event evidence.Event) []string {
	var named []string
	if event.RunID != "" {
		named = append(named, event.RunID)
	}
	if event.Proposed != nil && event.Proposed.Context != nil && event.Proposed.Context.RunID != "" {
		named = append(named, event.Proposed.Context.RunID)
	}
	return named
}

// writtenForRun reports whether a record may be read as this run's.
//
// An unstamped record cannot be placed either way, and is read as this run's
// because it was collected from this run's directory; whether the trail says
// which run wrote it is asserted once, by Evidence, rather than silently here.
// A record that names another run is never this run's: the run directory is
// fresh per run, and a directory is luck rather than an assertion.
func writtenForRun(named []string, runID string) bool {
	for _, id := range named {
		if id != runID {
			return false
		}
	}
	return true
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
