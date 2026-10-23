// Package check holds the assertions a scenario run is graded by.
//
// Each one reads records the run left behind — which services came up, what the
// agent could reach, the victims' journals — and reports what it established. None of them asks a running service how it is, and none
// of them reads the agent's own account of its work.
//
// The rule they share: an assertion over an empty set of records is
// indeterminate and never satisfied. "No envelope carries a preview" and "every
// decision carries a digest" are both true of a trail with nothing in it, and a
// check that reported those as passes would let a run that recorded nothing be
// read as green.
package check

import "github.com/guardana/playground/internal/journal"

// Spoken renders what a service or a probe said, naming silence rather than
// printing nothing, so a report never shows an empty reason as a reason.
func Spoken(detail string) string {
	if detail == "" {
		return "nothing"
	}
	return detail
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
