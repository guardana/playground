package main

import (
	"context"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/guardana/playground/drivers/control"
	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/dockercompose"
	"github.com/guardana/playground/runner/driver"
	"github.com/guardana/playground/runner/report"
)

// What the runner and the compose topology agree on. The runner is the only
// thing that knows a scenario's files by their repository paths, so it is the
// one that translates them into the paths a container sees.
const (
	agentService = "scripted-agent"
	// agentProfile holds the agent alone; no scenario names it, and a build
	// over the scenario's own profiles would skip the agent's image.
	agentProfile = "agent"
	servicePort  = "8080"
	// trajectoriesDir is the one directory of the workspace the agent mounts.
	trajectoriesDir = "trajectories/"
	// containerReports is where a service sees the run: /reports/<run id>,
	// holding only the part of the run that service writes. The agent gets
	// the directory it reads and not the repository: compose mounts
	// trajectories/ read only at its own name, so a repository path becomes a
	// container path by putting a slash in front of it. Handing the agent the
	// whole tree, attack payloads included, would be a strange thing for this
	// lab of all labs to do.
	containerReports = "/reports"
)

// inContainer is the path a service sees for a file the scenario names by its
// repository path. It holds only for the directories compose mounts at their
// own names; a scenario naming a file anywhere else is refused before a run.
func inContainer(repositoryPath string) string { return "/" + repositoryPath }

// lab runs one scenario end to end.
type lab struct {
	// root is the clone, which the lab's own files are read from; workspace is
	// where the scenario and every file it names are read from.
	root      string
	workspace workspace
	reports   string
	compose   dockercompose.Compose
	// timeout is how long one scenario may take. Zero is read as the default:
	// the rule is that every docker call carries a deadline, and a zero here
	// would be a deadline that has already passed.
	timeout time.Duration
	keep    bool
	clock   func() time.Time
	suffix  func() string
	// token mints a verifier run's bearer token; nil mints 32 random bytes.
	token func() (string, error)
	log   io.Writer
	// describe reads what produced the run for the report header; nil records
	// that nothing was read.
	describe func(context.Context) report.Provenance
	// control is the enforcer, the gate of every scenario with a gateway.
	control *control.Driver
	// development names a build a subject runs in place of its pin, for the
	// run's lines; empty when every subject runs at its pin.
	development string
	inspect     lookup
}

// execute runs one scenario and writes its reports. It returns an error only
// when the reports themselves could not be written: everything that went wrong
// inside the run is in the report, because a run that failed silently and a run
// that passed look the same to a caller that only reads errors.
func (l lab) execute(ctx context.Context, scenarioPath string) (assertion.Report, error) {
	ctx, cancel := context.WithTimeout(ctx, l.deadline())
	defer cancel()

	id := strings.TrimSuffix(filepath.Base(scenarioPath), filepath.Ext(scenarioPath))
	runID := l.mint(id)
	runDir := filepath.Join(l.reports, runID)
	if err := makeRunDir(l.reports, runDir); err != nil {
		return assertion.Report{}, err
	}

	provenance := l.provenance(ctx)
	spec, trajectory, err := load(l.workspace, scenarioPath, l.admit)
	if err != nil {
		// Nothing has been brought up, and nothing will be: a pair that does
		// not validate cannot be graded, and a run that cannot be graded is a
		// failure rather than a silence.
		graded := refused(id, runID, scenarioPath, err, l.clock())
		return graded, writeReports(runDir, graded, nil, provenance)
	}
	graded, rows := l.runScenario(ctx, spec, trajectory, runID, runDir)
	return graded, writeReports(runDir, graded, rows, provenance)
}

// prepare builds the run's environment: what the gate needs, when the
// scenario has one, and the token and the double's persona for a verifier run.
// The token is returned so the run can look for it after.
func (l lab) prepare(ctx context.Context, gate driver.Gate, spec labspec.Scenario, run driver.Run) (map[string]string, string, error) {
	env := l.environment(run)
	if gate != nil {
		added, err := gate.Prepare(ctx, run, spec)
		if err != nil {
			return nil, "", err
		}
		maps.Copy(env, added)
	}
	if err := l.handPersona(spec, run.Dir); err != nil {
		return nil, "", err
	}
	token, err := l.verifierEnv(spec, env)
	return env, token, err
}

func (l lab) runScenario(
	ctx context.Context, spec labspec.Scenario, trajectory labspec.Trajectory, runID, runDir string,
) (assertion.Report, []report.StepRow) {
	chosen := l.pick(spec)
	for _, subject := range chosen.subjects {
		if refusal := subject.Ready(ctx); refusal != nil {
			return subjectRefused(spec.ID, runID, *refusal, l.clock()), nil
		}
	}
	run := newRun(runID, runDir)
	env, token, err := l.prepare(ctx, chosen.gate, spec, run)
	if err != nil {
		return unprepared(spec.ID, runID, err, l.clock()), nil
	}
	run.Compose = l.compose.WithEnv(env)
	if !l.keep {
		defer l.takeDown(ctx, run, chosen.downProfiles(spec), chosen.gate)
	}
	inProfile, err := run.Compose.Services(ctx, spec.Profile)
	switch {
	case err != nil && ctx.Err() == nil:
		// A compose that cannot read the topology, an older one included, would
		// leave a boot with nothing observed; the report carries compose's words.
		return composeRefused(spec.ID, runID, err, l.clock()), nil
	case err != nil:
		// A run already out of time is not compose's failure: boot records that
		// nothing came up.
		l.note("listing the profile's services: %v", err)
	}
	// Taken before anything boots. assertion.Run times the checks, which are
	// the fast part; a report that said a run took no time because the reading
	// of its records took no time would be telling a reader the wrong thing
	// about where the minutes went.
	started := l.clock()
	boot := l.boot(ctx, run.Compose, spec, inProfile, runDir)

	var graded assertion.Report
	var rows []report.StepRow
	if chosen.prober != nil {
		graded = l.gradeVerifier(ctx, chosen.prober, run, spec, boot, token)
	} else {
		graded, rows = l.gradeTrajectory(ctx, chosen, run, spec, trajectory, boot)
	}
	graded.StartedAt, graded.EndedAt = started, l.clock()
	if spec.Gap != nil {
		graded.Gap = spec.Gap.Why
	}
	return graded, rows
}

// boot builds every image the profile uses, the agent's included, before the
// plane loads its policy, whose freshness a build under load would spend; then
// it brings up the long running services and records what came up. The agent
// and each subject's one-shots are run after the topology is proved.
func (l lab) boot(ctx context.Context, compose dockercompose.Compose, spec labspec.Scenario, inProfile []string, runDir string) assertion.Boot {
	boot := assertion.Boot{Profile: strings.Join(spec.Profile, ", ")}
	oneShots := l.oneShots()
	wanted := slices.DeleteFunc(slices.Clone(inProfile), func(name string) bool {
		return slices.Contains(oneShots, name)
	})
	if err := compose.Build(ctx, append(slices.Clone(spec.Profile), agentProfile)); err != nil {
		for _, name := range wanted {
			boot.Services = append(boot.Services, assertion.Service{Name: name, Detail: "not started, the build failed: " + err.Error()})
		}
		return l.writeBoot(runDir, boot)
	}
	if len(wanted) == 0 {
		// A profile of one-shots alone, as a scan's: compose up with no service
		// named would start them all.
		return l.writeBoot(runDir, boot)
	}
	upErr := compose.Up(ctx, spec.Profile, wanted)
	if upErr != nil {
		// Recorded rather than returned: what did come up is still a fact, and
		// the boot check reports the rest as not running, with this as why.
		l.note("bringing the profile up: %v", upErr)
	}
	status, err := compose.Status(ctx, spec.Profile, wanted)
	if err != nil {
		l.note("reading what came up: %v", err)
		return boot
	}
	for i := range status {
		if !status[i].Running && upErr != nil {
			status[i].Detail = strings.TrimSpace(status[i].Detail + "; up failed: " + upErr.Error())
		}
	}
	boot.Services = status
	return l.writeBoot(runDir, boot)
}

// takeDown brings the profile down and has the gate release what it
// prepared. It uses not the run's context: a scenario that ran out of time is
// the one whose containers are still up, and a cancelled context would leave
// them there.
func (l lab) takeDown(ctx context.Context, run driver.Run, profile []string, gate driver.Gate) {
	down, stop := context.WithTimeout(context.WithoutCancel(ctx), teardownTimeout)
	defer stop()
	if err := run.Compose.Down(down, profile); err != nil {
		l.note("taking the profile down: %v", err)
	}
	if gate == nil {
		return
	}
	if err := gate.Release(run); err != nil {
		l.note("%v", err)
	}
}

func (l lab) writeBoot(runDir string, boot assertion.Boot) assertion.Boot {
	if err := writeJSON(filepath.Join(runDir, "boot.json"), boot); err != nil {
		l.note("writing the boot record: %v", err)
	}
	return boot
}

func (l lab) mint(id string) string {
	return fmt.Sprintf("%s-%s-%s", id, l.clock().UTC().Format("20060102T150405Z"), l.suffix())
}

func (l lab) note(format string, values ...any) {
	if l.log == nil {
		return
	}
	_, _ = fmt.Fprintf(l.log, "runner: "+format+"\n", values...)
}

func lastLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
