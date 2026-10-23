package main

import (
	"context"
	"path"
	"path/filepath"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
	"github.com/guardana/playground/runner/driver"
	"github.com/guardana/playground/runner/report"
)

// gradeTrajectory replays the trajectory through the gate and grades what the
// gate recorded, the grader's analysis when there is one, and the victims'
// journals. Every trajectory scenario names a gateway, so the gate is set.
func (l lab) gradeTrajectory(
	ctx context.Context, chosen picked, run driver.Run, spec labspec.Scenario, trajectory labspec.Trajectory,
	boot assertion.Boot,
) (assertion.Report, []report.StepRow) {
	gate := chosen.gate.Begin(run, spec, trajectory)
	checks := []assertion.Check{
		check.Boot{Source: filepath.Join(run.Dir, "boot.json")}, l.isolation(ctx, run, spec, trajectory, gate),
	}
	flags := gate.AgentFlags()
	if chosen.grader != nil {
		flags = append(flags, chosen.grader.AgentFlags(run)...)
	}
	checks = append(checks, gate.Chaos(ctx, func(ctx context.Context) assertion.Check {
		return l.replay(ctx, run, spec, flags)
	})...)
	if chosen.grader != nil {
		checks = append(checks, chosen.grader.Analyze(ctx, run, spec))
	}
	checks = append(checks, gate.AfterReplay(ctx)...)
	// Every victim the profile booted answers for what it served, named or not.
	served := everyVictimServesNothing(spec, boot)
	records := l.collect(served, boot, run.ID, run.Dir)
	before, after := gate.Collect()
	checks = append(append(checks, before...), check.Effects{Scenario: served, JournalDir: filepath.Join(run.Dir, "journals")})
	graded := assertion.Run(ctx, records, append(checks, after...)...)
	return graded, gate.Rows()
}

// replay runs the trajectory by running the agent, and keeps what the agent
// printed. The exit status is the record; the agent's own log is not read.
func (l lab) replay(ctx context.Context, run driver.Run, spec labspec.Scenario, flags []string) check.Replay {
	source := filepath.Join(run.Dir, "replay.log")
	args := append([]string{
		"-trajectory", inContainer(spec.Trajectory),
		"-run-id", run.ID,
		"-out", path.Join(containerReports, run.ID, "agent", "agent.jsonl"),
	}, flags...)
	execution, err := run.Compose.RunOnce(ctx, spec.Profile, agentService, args)
	if writeErr := writeBytes(source, []byte(execution.Output)); writeErr != nil {
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
