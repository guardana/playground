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
	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/internal/runfile"
	"github.com/guardana/playground/runner/report"
)

// load reads the two files and checks the rules that need both of them,
// every file the scenario names read from the workspace; admit is the gate's
// own refusal of a pair it could not decide.
func load(
	space workspace, scenarioPath string, admit func(labspec.Scenario, labspec.Trajectory) error,
) (labspec.Scenario, labspec.Trajectory, error) {
	spec, err := labspec.LoadScenario(scenarioPath)
	if err != nil {
		return labspec.Scenario{}, labspec.Trajectory{}, err
	}
	if err := space.refuseMissing(spec); err != nil {
		return labspec.Scenario{}, labspec.Trajectory{}, err
	}
	if spec.IsVerifier() {
		// LoadScenario has checked every rule a verifier scenario has: there
		// is no trajectory for it to agree with.
		return spec, labspec.Trajectory{}, nil
	}
	trajectory, err := labspec.LoadTrajectory(space.file(spec.Trajectory))
	if err != nil {
		return labspec.Scenario{}, labspec.Trajectory{}, err
	}
	if err := labspec.Validate(spec, trajectory); err != nil {
		return labspec.Scenario{}, labspec.Trajectory{}, err
	}
	if err := admit(spec, trajectory); err != nil {
		return labspec.Scenario{}, labspec.Trajectory{}, err
	}
	if err := mountable(spec); err != nil {
		return labspec.Scenario{}, labspec.Trajectory{}, err
	}
	return spec, trajectory, nil
}

// mountable refuses a trajectory the agent cannot see. Compose mounts
// trajectories/ at its own name, so inContainer turns a repository path into a
// container path only for a file under it; any other path would hand the agent
// a file that is not there.
func mountable(spec labspec.Scenario) error {
	if spec.Trajectory != "" && !strings.HasPrefix(spec.Trajectory, trajectoriesDir) {
		return fmt.Errorf("trajectory is %q; the agent sees only %s, so it has to live there", spec.Trajectory, trajectoriesDir)
	}
	return nil
}

// collect reads the victims' journals the run left on disk.
//
// A journal that could not be read is left out of the map rather than entered
// as an empty one: a victim that never started and a victim that served nothing
// are the two answers a denial scenario has to tell apart, and only the
// presence of the file tells them apart.
func (l lab) collect(spec labspec.Scenario, boot assertion.Boot, runID, runDir string) assertion.Records {
	records := assertion.Records{
		RunID:    runID,
		Scenario: spec.ID,
		Boot:     boot,
		Journals: make(map[string][]journal.Entry, len(spec.Expect.Effects)),
		Auth:     map[string][]journal.Auth{},
		Unread:   map[string]string{},
	}
	for _, victim := range slices.Sorted(maps.Keys(spec.Expect.Effects)) {
		entries, err := journal.ReadServer(filepath.Join(runDir, "journals"), victim)
		if err != nil {
			l.note("reading %s's journal: %v", victim, err)
			if !errors.Is(err, fs.ErrNotExist) {
				records.Unread[victim] = err.Error()
			}
			continue
		}
		records.Journals[victim] = entries
		if len(spec.Expect.Effects[victim].AuthInclude) == 0 {
			continue
		}
		decisions, err := journal.ReadServerAuth(filepath.Join(runDir, "journals"), victim)
		if err != nil {
			records.Unread[victim] = err.Error()
			continue
		}
		records.Auth[victim] = decisions
	}
	return records
}

// What a run directory holds and who writes into it.
//
// The directory is one run's own scratch output: the evidence trail the
// collector exported, one journal per writer in a directory of its own, the
// boot and probe records, and the two reports. Nothing in it is secret — the
// fixtures are synthetic and the canary tokens are planted to be found — and
// reports/ is not tracked.
//
// The runner writes the run directory itself; compose bind-mounts only its
// subdirectories, and the services write there as nonroot, uid 65532, which on
// Linux is the uid on the mount, or a subordinate uid of the runner's user under
// rootless Docker. So the run directory and journals/ are the
// runner's at 0755, and each directory a service writes into is world-writable
// and sticky: the service can add its record, and another local user can neither
// remove nor replace one. The services share one uid, so the bit does not keep
// them from each other's files; a journal directory mounted into its writer
// alone does.
const (
	reportsMode = 0o755
	runDirMode  = 0o755
	sharedMode  = runfile.Shared
	serviceUID  = 65532
)

// makeRunDir creates the directory this run writes into, and refuses one that
// is already there. A run that wrote into an existing directory would be graded
// on whatever the last run left in it, which is the difference between reading
// a record and reading a record of something else.
func makeRunDir(reports, runDir string) error {
	if err := makeReports(reports); err != nil {
		return err
	}
	if err := runfile.MakeDir(runDir, runDirMode); err != nil {
		return err
	}
	if err := makeShared(filepath.Join(runDir, "agent")); err != nil {
		return err
	}
	journals := filepath.Join(runDir, "journals")
	if err := runfile.MakeDir(journals, runDirMode); err != nil {
		return err
	}
	// The double's persona directory, read-only to it and empty unless the
	// scenario names a persona: compose refuses a missing source.
	if err := runfile.MakeDir(filepath.Join(runDir, personaDir), runDirMode); err != nil {
		return err
	}
	// Every writer's directory, whatever the scenario boots: the run directory
	// is made before the scenario is read, and compose refuses a missing source.
	for _, writer := range labspec.JournalWriters() {
		if err := makeShared(filepath.Join(journals, writer)); err != nil {
			return err
		}
	}
	return nil
}

// makeReports creates the reports directory traversable, and leaves the mode
// of one that is already there to its owner.
func makeReports(reports string) error {
	if _, err := os.Stat(reports); !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(reports, reportsMode); err != nil {
		return err
	}
	return os.Chmod(reports, reportsMode)
}

// makeShared creates one directory every service can write into.
func makeShared(path string) error { return runfile.MakeDir(path, sharedMode) }

// refuseServiceUID refuses a runner with the uid every lab service runs as:
// the modes of the run directory keep nothing from a service that is its owner.
func refuseServiceUID(uid int) error {
	if uid == serviceUID {
		return fmt.Errorf("the runner runs as uid %d, the uid every lab service runs as; run it as another user", uid)
	}
	return nil
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

func writeReports(runDir string, graded assertion.Report, rows []report.StepRow, provenance report.Provenance) error {
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

func writeBytes(path string, body []byte) error {
	return write(path, func(file *os.File) error { _, err := file.Write(body); return err })
}

func write(path string, body func(*os.File) error) error { return runfile.CreateNew(path, 0o600, body) }
