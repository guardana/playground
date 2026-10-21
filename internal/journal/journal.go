// Package journal is the record a victim tool server keeps of the calls it
// received: those it served and those it refused.
//
// A scenario grades effects from this file and never from what the agent said
// it did. That is the whole reason the file exists: when the gateway denies a
// call, the proof is that the tool server has no line for it, written by the
// server rather than reported by the caller that was denied.
//
// One line per call, appended, never rewritten. The server stamps its own name
// on every line, so a caller cannot file a call under someone else's journal.
// A token-gated server also appends one auth line per request to its MCP
// endpoint (Auth), in the same file; an auth line is never a call.
package journal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// Status says what the server did with a call it received.
type Status string

const (
	// Served means the tool ran and the server answered with its result. It is
	// the only status a scenario's calls_served counts.
	Served Status = "served"
	// Refused means the server received the call and did not run the tool: bad
	// arguments, an unknown tool, a path outside its sandbox.
	Refused Status = "refused"
)

// Known reports whether a writer in this package accepts status, and so
// whether a reader can say what a line carrying it means.
func (s Status) Known() bool { return s == Served || s == Refused }

// MaxLineBytes is the longest journal line this package will read. A server
// writing past it makes its whole journal unreadable, and an effect assertion
// that cannot read a journal is one that establishes nothing — so the bound is
// exported for the writing side to stay under, and Detail is the only field
// whose length a caller controls.
const MaxLineBytes = 1 << 20

// MaxFileBytes is the largest journal this package will read: thirty-two
// lines at MaxLineBytes, where a catalogue run writes a few kilobytes.
const MaxFileBytes = 32 << 20

// fileMode lets the runner read a journal a server wrote as another uid, as it
// does on a Linux host. Only the server writes it; nothing in it is secret.
const fileMode = 0o644

var (
	// ErrInvalidEntry reports a line this package will not write, because it
	// would be a record nothing can be counted from.
	ErrInvalidEntry = errors.New("journal: invalid entry")

	// ErrLineTooLong reports a line past MaxLineBytes. It is reported rather
	// than skipped: a journal read short would undercount the calls a victim
	// served, which is the direction that hides a defect.
	ErrLineTooLong = errors.New("journal: line too long")
)

// Entry is one call a server received. Server is stamped by the writer and is
// ignored on the way in.
type Entry struct {
	OccurredAt time.Time `json:"occurred_at"`
	Server     string    `json:"server"`
	Tool       string    `json:"tool"`
	RunID      string    `json:"run_id,omitempty"`
	Status     Status    `json:"status"`
	// Detail is for a person reading a failed run: the path, the recipient, the
	// statement. It carries no fixture secret, because the fixtures are
	// synthetic and the canaries are planted to be found.
	Detail string `json:"detail,omitempty"`
	// Effect is what a served call changed, stated by a victim that commits
	// something a scenario grades by value: an amount, a destination. Absent
	// on every other line.
	Effect Effect `json:"effect,omitempty"`
}

// Writer appends entries to one server's journal. It is safe for concurrent
// use: a tool server answers calls concurrently, and two lines interleaved into
// one would undercount, which is the direction that hides a call.
type Writer struct {
	server string

	mu   sync.Mutex
	file *os.File
}

// Open creates or appends to the journal at path and stamps server on every
// entry written through it.
func Open(path, server string) (*Writer, error) {
	if server == "" {
		return nil, fmt.Errorf("%w: no server name", ErrInvalidEntry)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, fileMode) // #nosec G302,G304 -- the path is the journal the caller configured; see fileMode.
	if err != nil {
		return nil, err
	}
	// The umask narrows the mode at creation, and a journal the runner cannot
	// read is graded as one that was never written.
	if err := file.Chmod(fileMode); err != nil { // #nosec G302 -- see fileMode.
		return nil, errors.Join(err, file.Close())
	}
	return &Writer{server: server, file: file}, nil
}

// Record appends one entry. It refuses an entry that names no tool, carries a
// status this package does not know, or an effect on anything but a served
// call: a line nothing can be attributed to would let a served call go
// uncounted.
func (w *Writer) Record(entry Entry) error {
	if entry.Tool == "" {
		return fmt.Errorf("%w: no tool", ErrInvalidEntry)
	}
	if !entry.Status.Known() {
		return fmt.Errorf("%w: status is %q, want %q or %q", ErrInvalidEntry, entry.Status, Served, Refused)
	}
	entry.Server = w.server
	if entry.OccurredAt.IsZero() {
		return fmt.Errorf("%w: no time", ErrInvalidEntry)
	}
	if err := validEffect(entry); err != nil {
		return err
	}
	// Marshalled before the lock, so a slow encode does not hold up the server.
	// JSON escapes every control character, so one entry is always one line.
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return w.append(line)
}

// append writes one marshalled line under the lock.
func (w *Writer) append(line []byte) error {
	line = append(line, '\n')
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err := w.file.Write(line)
	return err
}

// validEffect holds an effect to a served line and to the bounds a reader
// accepts. An empty map is refused too: written, it would vanish under
// omitempty and the line would read as one that changed nothing.
func validEffect(entry Entry) error {
	if entry.Effect == nil {
		return nil
	}
	if entry.Status != Served {
		return fmt.Errorf("%w: an effect on a %s line", ErrInvalidEntry, entry.Status)
	}
	if err := entry.Effect.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidEntry, err)
	}
	return nil
}

// Close flushes the journal to disk and closes it. The sync is not a
// formality: the runner reads this file after the container it was written in
// has stopped.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return errors.Join(w.file.Sync(), w.file.Close())
}

// CountsByTool counts the calls a server served, by tool.
func CountsByTool(entries []Entry) map[string]int {
	counts := make(map[string]int)
	for _, entry := range entries {
		if entry.Status == Served {
			counts[entry.Tool]++
		}
	}
	return counts
}
