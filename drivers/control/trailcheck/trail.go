// Package trailcheck holds the checks that grade the enforcer's own records:
// its evidence trail, its /healthz counters and what it reports about itself.
// They share runner/check's rule: an assertion over an empty set of records is
// indeterminate and never satisfied.
package trailcheck

import (
	"errors"

	"github.com/guardana/playground/drivers/control/evidence"
)

// errUnread is a trail the checks were handed before anything read it.
var errUnread = errors.New("the evidence trail was never read")

// Trail is the enforcer's evidence trail for one run. The checks that grade it
// are built before the plane drains and hold it by pointer; the runner fills it
// once the export is on disk.
//
// The zero value is a trail nobody read, never an empty one: a check holding it
// reports that it could not read what it needed, so an empty trail cannot pass
// for one that was never collected.
type Trail struct {
	// Events is the whole trail, in the order it was written.
	Events []evidence.Event
	// ReadError is why the trail could not be read, when it could not be. A
	// trail that is not there leaves it nil: nothing wrote one, and a record
	// that should exist and does not is a failure, not an unreadable file.
	ReadError error
	// Read is set once the runner looked for the trail, whatever it found.
	Read bool
}

// events returns the trail, or errUnread when nobody looked for it.
func (t *Trail) events() ([]evidence.Event, error) {
	if t == nil || !t.Read {
		return nil, errUnread
	}
	return t.Events, nil
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
