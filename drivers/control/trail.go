package control

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/guardana/playground/drivers/control/evidence"
	"github.com/guardana/playground/drivers/control/trailcheck"
)

// maxEvents bounds one run's trail. A trail longer than this is refused rather
// than read short: reading the first n events of a longer file would grade a
// run on a prefix whose end nobody saw.
const maxEvents = 50000

// collectTrail reads the evidence trail. Its read error is kept rather than
// noted alone because a trail nobody could read and a trail with nothing in it
// are two facts: the second sends a reader to a file that turns out to be full.
func (d *Driver) collectTrail(runDir string) trailcheck.Trail {
	events, err := readTrail(filepath.Join(runDir, "evidence.jsonl"))
	trail := trailcheck.Trail{Events: events, Read: true}
	if err != nil {
		d.note("reading the evidence trail: %v", err)
		// A trail that is not there is not a trail nobody could read: nothing
		// wrote one, and a record that should exist and does not is a failure
		// the evidence check reports as such.
		if !errors.Is(err, fs.ErrNotExist) {
			trail.ReadError = err
		}
	}
	return trail
}

func readTrail(path string) ([]evidence.Event, error) {
	file, err := os.Open(path) // #nosec G304,G703 -- the path is inside the run directory the runner made.
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return evidence.DecodeJSONL(file, maxEvents)
}
