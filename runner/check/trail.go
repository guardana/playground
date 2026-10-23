package check

import (
	"errors"

	"github.com/guardana/playground/internal/evidence"
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
