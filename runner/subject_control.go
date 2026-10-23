package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/guardana/playground/drivers/control/trailcheck"
	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
	"github.com/guardana/playground/runner/driver"
	"github.com/guardana/playground/runner/report"
)

// controlSubject is the enforcer as the gate in front of the victims.
type controlSubject struct{ lab lab }

func newControl(l lab) controlSubject { return controlSubject{lab: l} }

// Ready refuses nothing: the enforcer's build is graded after the run, from
// what its running container reports.
func (controlSubject) Ready(context.Context) *driver.Refusal { return nil }

func (controlSubject) OneShots() []string { return nil }

// Describe reads the enforcer's image by the commit and the tree its build
// stamps, and names a development build used in place of the pin.
func (c controlSubject) Describe(ctx context.Context, inspect driver.Lookup, pins driver.Pins) driver.Description {
	name, commit := pins.Value("ENFORCER_IMAGE"), pins.Value("ENFORCER_COMMIT")
	image := inspectImage(ctx, inspect, name, commit, revisionLabel)
	image.TreeWant = or(pins.Value("ENFORCER_TREE"), "none in versions.env")
	if image.Missing == "" {
		tree := inspectImage(ctx, inspect, name, commit, treeLabel)
		image.Tree = or(tree.Missing, tree.Label)
	}
	described := driver.Description{Images: []report.Image{image}}
	if c.lab.development != nil {
		described.Development = c.lab.development.describe(commit)
	}
	return described
}

func (c controlSubject) Prepare(ctx context.Context, run driver.Run, spec labspec.Scenario) (map[string]string, error) {
	if err := c.lab.prepareEnforcer(ctx, spec, run.Dir); err != nil {
		return nil, err
	}
	return map[string]string{
		"LAB_PDP_SCRIPT":      spec.Gateway.PDPScript,
		"LAB_APPROVER_SCRIPT": spec.Gateway.ApproverScript,
	}, nil
}

func (controlSubject) Release(run driver.Run) error {
	if err := dropCollectorKey(run.Dir); err != nil {
		return fmt.Errorf("removing the collector's key: %w", err)
	}
	return nil
}

func (c controlSubject) Begin(run driver.Run, spec labspec.Scenario, trajectory labspec.Trajectory) driver.GateRun {
	return &controlRun{lab: c.lab, run: run, spec: spec, trajectory: trajectory, trail: &trailcheck.Trail{}}
}

// controlRun holds the enforcer's trail, which its checks share and Collect
// fills once the plane has drained.
type controlRun struct {
	lab        lab
	run        driver.Run
	spec       labspec.Scenario
	trajectory labspec.Trajectory
	trail      *trailcheck.Trail
}

func (r *controlRun) Isolation(ctx context.Context) assertion.Check {
	gateway, victim, sealed, probed := r.lab.probes(ctx, r.run.Compose, r.spec, r.trajectory, r.run.Dir)
	return check.NetworkIsolation{Gateway: gateway, Victim: victim, Sealed: sealed, Source: probed}
}

func (r *controlRun) AgentFlags() []string {
	return []string{"-gateway", "http://" + enforcerService + ":" + servicePort + "/mcp", "-namespace", r.lab.namespace}
}

// Chaos applies the scenario's faults, replays the trajectory and lifts the
// faults again, all before anything drains the trail.
func (r *controlRun) Chaos(ctx context.Context, replay func(context.Context) assertion.Check) []assertion.Check {
	if len(r.spec.Chaos) == 0 {
		return []assertion.Check{replay(ctx)}
	}
	applied := r.lab.applyChaos(ctx, r.run.Compose, r.spec)
	replayed := replay(ctx)
	graded := r.lab.liftChaos(ctx, r.run.Compose, r.spec, applied, r.run.Dir)
	graded.Scenario, graded.Trajectory, graded.Trail = r.spec, r.trajectory, r.trail
	return []assertion.Check{replayed, graded}
}

func (r *controlRun) AfterReplay(ctx context.Context) []assertion.Check {
	checks := []assertion.Check{
		r.lab.drainPlane(ctx, r.run.Compose, r.spec.Profile, r.lab.pin, r.run.Dir),
		trailcheck.TrailClaims{Scenario: r.spec, EvidenceFile: r.evidenceFile(), Trail: r.trail},
	}
	if r.spec.Expect.Health != nil {
		checks = append(checks, trailcheck.Health{Expect: *r.spec.Expect.Health, Source: filepath.Join(r.run.Dir, healthRecord)})
	}
	return checks
}

func (r *controlRun) Collect() (before, after []assertion.Check) {
	*r.trail = r.lab.collectTrail(r.run.Dir)
	evidenceFile := r.evidenceFile()
	before = []assertion.Check{
		trailcheck.Decisions{Scenario: r.spec, Trajectory: r.trajectory, EvidenceFile: evidenceFile, Trail: r.trail},
		trailcheck.Trails{Scenario: r.spec, Trajectory: r.trajectory, EvidenceFile: evidenceFile, Trail: r.trail},
	}
	after = []assertion.Check{trailcheck.Evidence{
		Scenario: r.spec, EvidenceFile: evidenceFile, Trail: r.trail,
		// The trail is read from the run's directory, which execute created
		// for this run and refuses when it already exists.
		FreshTrail: true,
	}}
	return before, after
}

func (r *controlRun) Rows() []report.StepRow {
	return trailcheck.DecisionRows(r.spec, r.trajectory, r.trail.Events, r.evidenceFile())
}

func (r *controlRun) evidenceFile() string { return filepath.Join(r.run.Dir, "evidence.jsonl") }
