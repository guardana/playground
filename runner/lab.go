package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
	"github.com/guardana/playground/runner/report"
)

// What the runner and the compose topology agree on. The runner is the only
// thing that knows a scenario's files by their repository paths, so it is the
// one that translates them into the paths a container sees.
const (
	agentService    = "scripted-agent"
	gatewayService  = "stub-gateway"
	servicePort     = "8080"
	gatewayEndpoint = "http://" + gatewayService + ":" + servicePort + "/mcp"
	// containerReports is where a service sees the run: /reports/<run id>,
	// holding only the part of the run that service writes. The agent and the
	// gateway get the two directories they read and not the repository:
	// compose mounts config/ and trajectories/ read only, at mount points
	// named after the directories themselves, so a repository path becomes a
	// container path by putting a slash in front of it. Handing a service
	// standing in for a security boundary the whole tree, attack payloads
	// included, would be a strange thing for this lab of all labs to do.
	containerReports = "/reports"
)

// inContainer is the path a service sees for a file the scenario names by its
// repository path. It holds only for the directories compose mounts at their
// own names; a scenario naming a file anywhere else is refused before a run.
func inContainer(repositoryPath string) string { return "/" + repositoryPath }

// lab runs one scenario end to end.
type lab struct {
	root    string
	reports string
	compose Compose
	// timeout is how long one scenario may take. Zero is read as the default:
	// the rule is that every docker call carries a deadline, and a zero here
	// would be a deadline that has already passed.
	timeout time.Duration
	keep    bool
	clock   func() time.Time
	suffix  func() string
	log     io.Writer
	// describe reads what produced the run for the report header; nil records
	// that nothing was read.
	describe func(context.Context) report.Provenance
	// namespace is the enforcer's, under which its gateway marks the answers
	// it makes itself; the agent reads a pending answer only under it.
	namespace string
	// pin is ENFORCER_COMMIT, which the running enforcer has to report.
	pin string
	// keysDir holds the lab key; sign signs a scenario's policy with it.
	keysDir string
	sign    signer
	// drainBound bounds the wait for the plane's trail; zero reads as the
	// default in drain.go.
	drainBound time.Duration
	// enforcerImage is ENFORCER_IMAGE:ENFORCER_COMMIT; inspect reads it.
	enforcerImage string
	inspect       lookup
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
	spec, trajectory, err := load(l.root, scenarioPath)
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

func (l lab) runScenario(
	ctx context.Context, spec labspec.Scenario, trajectory labspec.Trajectory, runID, runDir string,
) (assertion.Report, []check.DecisionRow) {
	if spec.IsVerifier() || spec.Trace != nil {
		if err := l.refuseVerifierImage(ctx); err != nil {
			return imageRefused(spec.ID, runID, err, l.clock()), nil
		}
	}
	env := l.environment(spec, runID, runDir)
	if spec.UsesEnforcer() {
		if err := l.prepareEnforcer(ctx, spec, runDir); err != nil {
			return unprepared(spec.ID, runID, err, l.clock()), nil
		}
		env["LAB_PDP_SCRIPT"] = spec.Gateway.PDPScript
		env["LAB_APPROVER_SCRIPT"] = spec.Gateway.ApproverScript
	}
	compose := l.compose.WithEnv(env)
	// Taken before anything boots. assertion.Run times the checks, which are
	// the fast part; a report that said a run took no time because the reading
	// of its records took no time would be telling a reader the wrong thing
	// about where the minutes went.
	started := l.clock()
	boot := l.boot(ctx, compose, spec, runDir)
	if !l.keep {
		defer func() {
			// Not this run's context: a scenario that ran out of time is the
			// one whose containers are still up, and a cancelled context would
			// leave them there.
			down, stop := context.WithTimeout(context.WithoutCancel(ctx), teardownTimeout)
			defer stop()
			if err := compose.Down(down, spec.Profile); err != nil {
				l.note("taking the profile down: %v", err)
			}
		}()
	}

	var graded assertion.Report
	var rows []check.DecisionRow
	if spec.IsVerifier() {
		graded = l.gradeVerifier(ctx, compose, spec, boot, runID, runDir)
	} else {
		graded, rows = l.gradeTrajectory(ctx, compose, spec, trajectory, boot, runID, runDir)
	}
	graded.StartedAt, graded.EndedAt = started, l.clock()
	if spec.Gap != nil {
		graded.Gap = spec.Gap.Why
	}
	return graded, rows
}

// boot brings up every long running service in the profile and records what
// came up. The agent and the verifier are left out: they are one-shots the
// runner drives itself, and starting one here would run it before the
// topology has been proved.
func (l lab) boot(ctx context.Context, compose Compose, spec labspec.Scenario, runDir string) assertion.Boot {
	boot := assertion.Boot{Profile: strings.Join(spec.Profile, ", ")}
	inProfile, err := compose.Services(ctx, spec.Profile)
	if err != nil {
		l.note("listing the profile's services: %v", err)
		return boot
	}
	wanted := slices.DeleteFunc(slices.Clone(inProfile), func(name string) bool {
		return name == agentService || name == verifierService || name == traceService
	})
	if err := compose.Up(ctx, spec.Profile, wanted); err != nil {
		// Recorded rather than returned: what did come up is still a fact, and
		// the boot check reports the rest as not running.
		l.note("bringing the profile up: %v", err)
	}
	status, err := compose.Status(ctx, spec.Profile, wanted)
	if err != nil {
		l.note("reading what came up: %v", err)
		return boot
	}
	boot.Services = status
	if err := writeJSON(filepath.Join(runDir, "boot.json"), boot); err != nil {
		l.note("writing the boot record: %v", err)
	}
	return boot
}

// probes asks, from inside agent-net, whether the gateway answers and one
// victim does not. Both answers are recorded, in memory for the check and on
// disk for the person the check sends to a file. Neither is inferred from the
// other: a topology where nothing at all is reachable would otherwise look like
// a topology that is right about the victim.
func (l lab) probes(
	ctx context.Context, compose Compose, spec labspec.Scenario, trajectory labspec.Trajectory, runDir string,
) (check.Probe, check.Probe, []check.Probe, string) {
	gateway := l.probe(ctx, compose, spec.Profile, gatewayHost(spec)+":"+servicePort)
	victim := l.probe(ctx, compose, spec.Profile, trajectory.Steps[0].Call.Server+":"+servicePort)
	var sealed []check.Probe
	for _, target := range sealedFromAgent(spec) {
		sealed = append(sealed, l.probe(ctx, compose, spec.Profile, target))
	}

	source := filepath.Join(runDir, "probes.log")
	recorded := fmt.Sprintf("gateway %s ran=%t reached=%t %s\nvictim %s ran=%t reached=%t %s\n",
		gateway.Target, gateway.Ran, gateway.Reached, gateway.Detail,
		victim.Target, victim.Ran, victim.Reached, victim.Detail)
	for _, probe := range sealed {
		recorded += fmt.Sprintf("sealed %s ran=%t reached=%t %s\n", probe.Target, probe.Ran, probe.Reached, probe.Detail)
	}
	// #nosec G703 -- the path is inside the run directory the runner made.
	if err := os.WriteFile(source, []byte(recorded), 0o600); err != nil {
		l.note("writing the probe record: %v", err)
	}
	return gateway, victim, sealed, source
}

func (l lab) probe(ctx context.Context, compose Compose, profiles []string, target string) check.Probe {
	execution, err := compose.RunOnce(ctx, profiles, agentService, []string{"-probe", target})
	if err != nil {
		return check.Probe{Target: target, Detail: err.Error()}
	}
	return readProbe(target, execution.Output)
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
