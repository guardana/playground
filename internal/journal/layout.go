package journal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// File is where server's journal sits under a run's journals directory: in a
// directory of the server's own, which compose mounts into that server alone.
func File(dir, server string) string {
	return filepath.Join(dir, server, server+".jsonl")
}

// ReadServer reads server's journal from its own directory under dir. A
// directory holding any other entry is refused, named, before the journal is
// read: nothing the lab runs writes one there, so the journal beside it is not
// the record its server alone kept. A missing journal is ReadFile's error.
func ReadServer(dir, server string) ([]Entry, error) {
	own := filepath.Join(dir, server)
	handle, err := os.Open(own) // #nosec G304 -- the directory is inside the run directory the runner made.
	if err != nil {
		return nil, err
	}
	// Two entries settle it: names in a directory are distinct, so a second
	// one is never the journal.
	entries, err := handle.ReadDir(2)
	if err := errors.Join(ignoreEOF(err), handle.Close()); err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Name() != server+".jsonl" {
			return nil, fmt.Errorf("%s holds %q, and only %s.jsonl belongs there", own, entry.Name(), server)
		}
	}
	return ReadFile(File(dir, server))
}

// ignoreEOF drops the io.EOF ReadDir answers for a directory with no entry.
func ignoreEOF(err error) error {
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
