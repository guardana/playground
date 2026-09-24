package main

import (
	"context"
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
	gateway, victim, probed := l.probes(ctx, compose, spec, trajectory, runDir)
	replayed := l.replay(ctx, compose, spec, runID, runDir)
	records, unreadable := l.collect(spec, boot, runID, runDir)

	trail := filepath.Join(runDir, "evidence.jsonl")
	graded := assertion.Run(ctx, records,
		check.Boot{Source: filepath.Join(runDir, "boot.json")},
		check.NetworkIsolation{Gateway: gateway, Victim: victim, Source: probed},
		replayed,
		check.Decisions{Scenario: spec, Trajectory: trajectory, EvidenceFile: trail},
		check.Trails{Scenario: spec, Trajectory: trajectory, EvidenceFile: trail},
		check.Effects{Scenario: spec, JournalDir: filepath.Join(runDir, "journals")},
		check.Evidence{
			Scenario: spec, EvidenceFile: trail, ReadError: unreadable,
			Unstamped: spec.Stub.Verdicts == "",
			// The trail is read from runDir, which execute created for this run
			// and refuses when it already exists.
			FreshTrail: true,
		},
	)
	return graded, check.DecisionRows(spec, trajectory, records, trail)
}
