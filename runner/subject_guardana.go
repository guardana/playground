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

// guardanaSubject is the verifier, as the prober of the victims and as the
// grader of the agent's trace.
type guardanaSubject struct{ lab lab }

func newGuardana(l lab) guardanaSubject { return guardanaSubject{lab: l} }

func (g guardanaSubject) Ready(ctx context.Context) *driver.Refusal {
	if err := g.lab.refuseVerifierImage(ctx); err != nil {
		return &driver.Refusal{Check: "verifier/image", Want: "the verifier's image built from VERIFIER_VERSION", Cause: err}
	}
	return nil
}

func (guardanaSubject) OneShots() []string {
	return []string{verifierService, traceService, scanService}
}

// Describe reads the verifier's image by the release its build stamps.
func (guardanaSubject) Describe(ctx context.Context, inspect driver.Lookup, pins driver.Pins) driver.Description {
	image := inspectImage(ctx, inspect, pins.Value("VERIFIER_IMAGE"), pins.Value("VERIFIER_VERSION"), versionLabel)
	return driver.Description{Images: []report.Image{image}}
}

// DownProfiles names the pin step's profile: a pin step that outlived its
// deadline would otherwise be left on tool-net when the scenario's own
// profile comes down.
func (guardanaSubject) DownProfiles() []string { return []string{verifierPinProfile} }

// Probe makes the verifier's directories before any verifier container
// starts, since compose binds directories in them and refuses to create them.
func (g guardanaSubject) Probe(
	ctx context.Context, run driver.Run, spec labspec.Scenario, token string,
) (assertion.Check, []check.VerifierRun) {
	dir := filepath.Join(run.Dir, verifierDir)
	made := makeDirs(dir, filepath.Join(dir, reachDir), filepath.Join(dir, reachDir, inputsDir))
	if made != nil {
		g.lab.note("making the verifier's directories: %v", made)
	}
	reach := g.lab.verifierReach(ctx, run.Compose.WithEnv(map[string]string{stepVariable: reachDir}), spec, run.Dir)
	runs := g.lab.runVerifier(ctx, run.Compose, spec, dir, made, token)
	readRecords(runs)
	return reach, runs
}

func (guardanaSubject) AgentFlags(run driver.Run) []string {
	return []string{"-trace", path.Join(containerReports, run.ID, "agent", traceFile)}
}

func (g guardanaSubject) Analyze(ctx context.Context, run driver.Run, spec labspec.Scenario) assertion.Check {
	return g.lab.analyzeTrace(ctx, run.Compose, spec, run.ID, run.Dir)
}
