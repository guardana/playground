package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
	"unicode/utf8"
)

// fileMode lets the runner read a journal the double wrote as another uid.
// Nothing in it is secret: header values are never written.
const fileMode = 0o644

// Long request values are cut, so one line stays far under what a reader
// accepts even with every byte escaped.
const (
	maxMethodBytes = 32
	maxPathBytes   = 256
)

// line is one request the double received and the status it answered. The
// two headers are recorded as present or absent and never by value: a bearer
// token the verifier sent must not land in a file the run keeps.
type line struct {
	OccurredAt    time.Time `json:"occurred_at"`
	Server        string    `json:"server"`
	RunID         string    `json:"run_id"`
	Method        string    `json:"method"`
	Path          string    `json:"path"`
	Authorization bool      `json:"authorization"`
	McpSessionID  bool      `json:"mcp_session_id"`
	// Query is whether the request carried a query string, and Headers the
	// names of the headers it carried, sorted and comma-separated: a
	// credential can ride in either, and neither value is ever written.
	Query   bool   `json:"query"`
	Headers string `json:"headers"`
	Status  int    `json:"status"`
	// Persona is the persona the double serves, absent when none loaded.
	Persona string `json:"persona,omitempty"`
}

// journalFile appends lines to the double's journal. It is safe for
// concurrent use, so two requests never interleave into one line.
type journalFile struct {
	mu   sync.Mutex
	file *os.File
}

func openJournal(path string) (*journalFile, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, fileMode) // #nosec G302,G304 -- the run's journal; see fileMode.
	if err != nil {
		return nil, err
	}
	// The umask narrows the mode at creation, and a journal the runner cannot
	// read is graded as one that was never written.
	if err := file.Chmod(fileMode); err != nil { // #nosec G302 -- see fileMode.
		return nil, errors.Join(err, file.Close())
	}
	return &journalFile{file: file}, nil
}

func (j *journalFile) record(entry line) error {
	if entry.OccurredAt.IsZero() || entry.Server == "" || entry.Status == 0 {
		return fmt.Errorf("auth-double: refusing a journal line with no time, server or status")
	}
	entry.Method = cut(entry.Method, maxMethodBytes)
	entry.Path = cut(entry.Path, maxPathBytes)
	body, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	body = append(body, '\n')
	j.mu.Lock()
	defer j.mu.Unlock()
	_, err = j.file.Write(body)
	return err
}

// close flushes the journal: the runner reads it after the container stopped.
func (j *journalFile) close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return errors.Join(j.file.Sync(), j.file.Close())
}

// cut keeps at most limit bytes of s, ending on a whole character, and says
// how long s was when it had to cut.
func cut(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	end := limit
	for end > limit-utf8.UTFMax && !utf8.RuneStart(s[end]) {
		end--
	}
	return fmt.Sprintf("%s [cut from %d bytes]", s[:end], len(s))
}
