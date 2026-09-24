// Package journal is the record a victim tool server keeps of the calls it
// actually served.
//
// A scenario grades effects from this file and never from what the agent said
// it did. That is the whole reason the file exists: when the gateway denies a
// call, the proof is that the tool server has no line for it, written by the
// server rather than reported by the caller that was denied.
//
// One line per call, appended, never rewritten. The server stamps its own name
// on every line, so a caller cannot file a call under someone else's journal.
package journal

import (
	"bufio"
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

// MaxLineBytes is the longest journal line this package will read. A server
// writing past it makes its whole journal unreadable, and an effect assertion
// that cannot read a journal is one that establishes nothing — so the bound is
// exported for the writing side to stay under, and Detail is the only field
// whose length a caller controls.
const MaxLineBytes = 1 << 20

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
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- the path is the journal the caller configured.
	if err != nil {
		return nil, err
	}
	return &Writer{server: server, file: file}, nil
}

// Record appends one entry. It refuses an entry that names no tool or carries a
// status this package does not know: a line nothing can be attributed to would
// let a served call go uncounted.
func (w *Writer) Record(entry Entry) error {
	if entry.Tool == "" {
		return fmt.Errorf("%w: no tool", ErrInvalidEntry)
	}
	if entry.Status != Served && entry.Status != Refused {
		return fmt.Errorf("%w: status is %q, want %q or %q", ErrInvalidEntry, entry.Status, Served, Refused)
	}
	entry.Server = w.server
	if entry.OccurredAt.IsZero() {
		return fmt.Errorf("%w: no time", ErrInvalidEntry)
	}
	// Marshalled before the lock, so a slow encode does not hold up the server.
	// JSON escapes every control character, so one entry is always one line.
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	w.mu.Lock()
	defer w.mu.Unlock()
	_, err = w.file.Write(line)
	return err
}

// Close flushes the journal to disk and closes it. The sync is not a
// formality: the runner reads this file after the container it was written in
// has stopped.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return errors.Join(w.file.Sync(), w.file.Close())
}

// ReadFile reads one server's journal. A journal that does not exist is an
// error and never an empty one: a server that never started would otherwise
// look exactly like a server that served nothing, and those are the two answers
// a denial scenario has to tell apart.
func ReadFile(path string) ([]Entry, error) {
	file, err := os.Open(path) // #nosec G304 -- the path is the journal the caller asked for.
	if err != nil {
		return nil, err
	}
	// Nothing was written, so a close error says nothing a caller could act on.
	defer func() { _ = file.Close() }()

	var entries []Entry
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64<<10), MaxLineBytes)
	line := 1
	for ; scanner.Scan(); line++ {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, line, err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			// Named, because the caller's next move is to open that line, and
			// "token too long" says neither which line nor how long is too long.
			return nil, fmt.Errorf("%s line %d: %w: limit %d bytes", path, line, ErrLineTooLong, MaxLineBytes)
		}
		return nil, fmt.Errorf("%s line %d: %w", path, line, err)
	}
	return entries, nil
}

// CountsByTool counts the calls a server served, by tool. A refused call is not
// counted: the scenario asks what the victim did, not what it was asked to do.
func CountsByTool(entries []Entry) map[string]int {
	counts := make(map[string]int)
	for _, entry := range entries {
		if entry.Status == Served {
			counts[entry.Tool]++
		}
	}
	return counts
}
