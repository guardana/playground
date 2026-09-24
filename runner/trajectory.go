package main

import (
	"context"
	"os"
	"path"
	"path/filepath"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

// gradeTrajectory replays the trajectory through the gateway and grades the
// decisions, the trail and the victims' journals.
func (l lab) gradeTrajectory(
	ctx context.Context, compose Compose, spec labspec.Scenario, trajectory labspec.Trajectory,
	boot assertion.Boot, runID, runDir string,
) (assertion.Report, []check.DecisionRow) {
	gateway, victim, sealed, probed := l.probes(ctx, compose, spec, trajectory, runDir)
	replayed := l.replay(ctx, compose, spec, runID, runDir)
	checks := []assertion.Check{
		check.Boot{Source: filepath.Join(runDir, "boot.json")},
		check.NetworkIsolation{Gateway: gateway, Victim: victim, Sealed: sealed, Source: probed},
		replayed,
	}
	if spec.Trace != nil {
		checks = append(checks, l.analyzeTrace(ctx, compose, spec, runID, runDir))
	}
	if spec.UsesEnforcer() {
		checks = append(checks, l.drainPlane(ctx, compose, spec.Profile, l.pin, runDir),
			check.TrailClaims{Scenario: spec, EvidenceFile: filepath.Join(runDir, "evidence.jsonl")})
	}
	records, unreadable := l.collect(spec, boot, runID, runDir)

	trail := filepath.Join(runDir, "evidence.jsonl")
	graded := assertion.Run(ctx, records, append(checks,
		check.Decisions{Scenario: spec, Trajectory: trajectory, EvidenceFile: trail},
		check.Trails{Scenario: spec, Trajectory: trajectory, EvidenceFile: trail},
		check.Effects{Scenario: spec, JournalDir: filepath.Join(runDir, "journals")},
		check.Evidence{
			Scenario: spec, EvidenceFile: trail, ReadError: unreadable,
			Unstamped: spec.UsesEnforcer(),
			// The trail is read from runDir, which execute created for this run
			// and refuses when it already exists.
			FreshTrail: true,
		},
	)...)
	return graded, check.DecisionRows(spec, trajectory, records, trail)
}

// replay runs the trajectory by running the agent, and keeps what the agent
// printed. The exit status is the record; the agent's own log is not read.
func (l lab) replay(
	ctx context.Context, compose Compose, spec labspec.Scenario, runID, runDir string,
) check.Replay {
	source := filepath.Join(runDir, "replay.log")
	args := []string{
		"-trajectory", inContainer(spec.Trajectory),
		"-gateway", "http://" + gatewayHost(spec) + ":" + servicePort + "/mcp",
		"-run-id", runID,
		"-namespace", l.namespace,
		"-out", path.Join(containerReports, runID, "agent", "agent.jsonl"),
	}
	if spec.Trace != nil {
		args = append(args, "-trace", path.Join(containerReports, runID, "agent", traceFile))
	}
	execution, err := compose.RunOnce(ctx, spec.Profile, agentService, args)
	// #nosec G703 -- the path is inside the run directory the runner made.
	if writeErr := os.WriteFile(source, []byte(execution.Output), 0o600); writeErr != nil {
		l.note("writing what the agent printed: %v", writeErr)
	}
	if err != nil {
		return check.Replay{Detail: err.Error(), Source: source}
	}
	return check.Replay{
		Ran:      true,
		ExitCode: execution.ExitCode,
		Detail:   lastLine(execution.Output),
		Source:   source,
	}
}
