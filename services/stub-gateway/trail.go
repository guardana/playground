package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/guardana/playground/internal/evidence"
)

// wireSchemaVersion is the version of the evidence contract these events are
// written under. It is the producer's statement about its own records, not a
// version of this stub.
const wireSchemaVersion = "1.0"

// runIdentity is what every event in one run carries, whichever call produced
// it. The project and tenant come from the declared verdict file, because the
// scenario author is the one who says which tenant the run belongs to.
type runIdentity struct {
	runID     string
	projectID string
	tenantID  string
}

// trail appends evidence events to one file, one JSON object per line.
//
// It is safe for concurrent use: calls arrive concurrently and two events
// interleaved into one line would be a trail nothing can read.
type trail struct {
	identity runIdentity

	mu  sync.Mutex
	out io.Writer
}

func newTrail(out io.Writer, identity runIdentity) *trail {
	return &trail{identity: identity, out: out}
}

// request starts one call's chain of events. Every event on it carries the same
// requestId and links to the one before it, so a gap in the account shows.
func (t *trail) request() *requestTrail {
	return &requestTrail{trail: t, id: newID("req")}
}

type requestTrail struct {
	trail *trail
	id    string
	prev  string
}

// requestID is what the caller records elsewhere to tie a decision to this
// call.
func (r *requestTrail) requestID() string { return r.id }

// append writes one event, stamping the identity every event carries and the
// link to the one before it on this request.
//
// The caller sets the kind and the one payload field that kind carries;
// everything an event has to say about which run it belongs to is filled in
// here, so no caller can write a record that names a different run.
func (r *requestTrail) append(event evidence.Event) error {
	event.SchemaVersion = wireSchemaVersion
	event.RequestID = r.id
	event.RunID = r.trail.identity.runID
	event.ProjectID = r.trail.identity.projectID
	event.TenantID = r.trail.identity.tenantID
	event.OccurredAt = time.Now().UTC()
	event.EventID = newID("evt")
	event.PrevEventID = r.prev

	// Encoded before the lock, and JSON escapes every control character, so one
	// event is always one line.
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	r.trail.mu.Lock()
	defer r.trail.mu.Unlock()
	if _, err := r.trail.out.Write(line); err != nil {
		return err
	}
	r.prev = event.EventID
	return nil
}

// newID mints an identifier unique to this process run. The prefix says what
// kind of thing it names, for a person reading the trail with no code to hand.
func newID(prefix string) string {
	var raw [16]byte
	// rand.Read fills the slice or terminates the process; there is no partial
	// read to handle.
	_, _ = rand.Read(raw[:])
	return prefix + "_" + hex.EncodeToString(raw[:])
}
