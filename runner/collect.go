package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
	"github.com/guardana/playground/runner/report"
)

// maxEvents bounds one run's trail. A trail longer than this is refused rather
// than read short: reading the first n events of a longer file would grade a
// run on a prefix whose end nobody saw.
const maxEvents = 50000

// load reads the two files and checks the rules that need both of them.
func load(root, scenarioPath string) (labspec.Scenario, labspec.Trajectory, error) {
	spec, err := labspec.LoadScenario(scenarioPath)
	if err != nil {
		return labspec.Scenario{}, labspec.Trajectory{}, err
	}
	trajectory, err := labspec.LoadTrajectory(filepath.Join(root, spec.Trajectory))
	if err != nil {
		return labspec.Scenario{}, labspec.Trajectory{}, err
	}
	if err := labspec.Validate(spec, trajectory); err != nil {
		return labspec.Scenario{}, labspec.Trajectory{}, err
	}
	if err := mountable(spec); err != nil {
		return labspec.Scenario{}, labspec.Trajectory{}, err
	}
	return spec, trajectory, nil
}

// mountable refuses a scenario naming a file the containers cannot see.
//
// The agent and the gateway get two directories, mounted at their own names, so
// inContainer turns a repository path into a container path by putting a slash
// in front of it. That holds only for a file under one of those two
// directories, and a scenario naming one elsewhere would otherwise boot a
// gateway pointed at a path that is not there — which is how the first run of
// this lab failed, with the gateway exiting before it could write a single
// evidence record.
func mountable(spec labspec.Scenario) error {
	for _, named := range []struct{ field, value, directory string }{
		{"trajectory", spec.Trajectory, "trajectories/"},
		{"stub.verdicts", spec.Stub.Verdicts, "config/"},
	} {
		if named.value == "" {
			continue
		}
		if !strings.HasPrefix(named.value, named.directory) {
			return fmt.Errorf("%s is %q; the containers see only %s, so it has to live there",
				named.field, named.value, named.directory)
		}
	}
	return nil
}

// collect reads what the run left on disk. The error it returns is why the
// evidence trail could not be read, and it is returned rather than noted
// because a trail nobody could read and a trail with nothing in it are two
// facts: the second sends a reader to a file that turns out to be full.
//
// A journal that could not be read is left out of the map rather than entered
// as an empty one: a victim that never started and a victim that served nothing
// are the two answers a denial scenario has to tell apart, and only the
// presence of the file tells them apart.
func (l lab) collect(spec labspec.Scenario, boot assertion.Boot, runID, runDir string) (assertion.Records, error) {
	records := assertion.Records{
		RunID:    runID,
		Scenario: spec.ID,
		Boot:     boot,
		Journals: make(map[string][]journal.Entry, len(spec.Expect.Effects)),
	}
	events, err := readTrail(filepath.Join(runDir, "evidence.jsonl"))
	var unreadable error
	if err != nil {
		l.note("reading the evidence trail: %v", err)
		// A trail that is not there is not a trail nobody could read: nothing
		// wrote one, and a record that should exist and does not is a failure
		// the evidence check reports as such.
		if !errors.Is(err, fs.ErrNotExist) {
			unreadable = err
		}
	}
	records.Evidence = events

	for _, victim := range slices.Sorted(maps.Keys(spec.Expect.Effects)) {
		entries, err := journal.ReadFile(filepath.Join(runDir, "journals", victim+".jsonl"))
		if err != nil {
			l.note("reading %s's journal: %v", victim, err)
			continue
		}
		records.Journals[victim] = entries
	}
	return records, unreadable
}

// What a run directory holds and who writes into it.
//
// The directory is one run's own scratch output: the evidence trail the stub
// gateway wrote, one journal per victim, the boot and probe records, and the
// two reports. Nothing in it is secret — the fixtures are synthetic and the
// canary tokens are planted to be found — and reports/ is not tracked.
//
// It is written by the lab services, not by the runner: compose bind-mounts
// reports/ into every one of them and they run as nonroot, uid 65532. On Linux
// that uid is the uid on the mount, so a directory this user owns at 0750 is
// one no service can write its record into; macOS hides it because Docker
// Desktop maps every access back to the invoking user. Hence the mode: a run's
// scratch directory is world-writable on purpose, and reports/ above it is
// traversable so a service can reach it.
const (
	reportsMode = 0o755
	runDirMode  = 0o777
)

// makeRunDir creates the directory this run writes into, and refuses one that
// is already there. A run that wrote into an existing directory would be graded
// on whatever the last run left in it, which is the difference between reading
// a record and reading a record of something else.
func makeRunDir(reports, runDir string) error {
	if err := os.MkdirAll(reports, reportsMode); err != nil {
		return err
	}
	if err := os.Chmod(reports, reportsMode); err != nil {
		return err
	}
	if err := makeShared(runDir); err != nil {
		return err
	}
	return makeShared(filepath.Join(runDir, "journals"))
}

// makeShared creates one directory the services can write into. The mode is set
// twice because Mkdir's is masked by the umask of whoever ran the runner, and a
// umask of 022 is the difference between a lab that records and one that does
// not.
func makeShared(path string) error {
	if err := os.Mkdir(path, runDirMode); err != nil { // #nosec G301,G703 -- see the mode's own comment.
		return err
	}
	return os.Chmod(path, runDirMode) // #nosec G302,G703 -- as above.
}

func readTrail(path string) ([]evidence.Event, error) {
	file, err := os.Open(path) // #nosec G304,G703 -- the path is inside the run directory the runner made.
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return evidence.DecodeJSONL(file, maxEvents)
}

// refused is the report of a scenario that never ran. It is a failure and not
// an absence: a pair of files that do not agree is a defect in the lab's own
// inputs, and a build that skipped it would be green on a scenario nobody ran.
func refused(id, runID, scenarioPath string, cause error, at time.Time) assertion.Report {
	return assertion.Report{
		Scenario:  id,
		RunID:     runID,
		StartedAt: at,
		EndedAt:   at,
		Results: []assertion.Result{{
			Check:   "scenario/loads",
			Outcome: assertion.Fail,
			Want:    "a scenario and a trajectory that load and agree",
			Got:     "the pair was refused before anything was brought up",
			Source:  scenarioPath,
			Detail:  cause.Error(),
		}},
	}
}

func writeReports(runDir string, graded assertion.Report, rows []check.DecisionRow, provenance report.Provenance) error {
	return errors.Join(
		write(filepath.Join(runDir, "junit.xml"), func(file *os.File) error {
			return report.WriteJUnit(file, graded, provenance)
		}),
		write(filepath.Join(runDir, "report.md"), func(file *os.File) error {
			return report.WriteMarkdown(file, graded, rows, provenance)
		}),
	)
}

func writeJSON(path string, value any) error {
	return write(path, func(file *os.File) error {
		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	})
}

func write(path string, body func(*os.File) error) error {
	// #nosec G304,G703 -- the path is inside the run directory the runner made.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	return errors.Join(body(file), file.Close())
}
