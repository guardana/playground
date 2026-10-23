package control

import (
	"context"
	"fmt"
	"strings"

	"github.com/guardana/playground/drivers/control/trailcheck"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/driver"
	"github.com/guardana/playground/runner/report"
)

// Ready refuses nothing: the enforcer's build is graded after the run, from
// what its running container reports.
func (*Driver) Ready(context.Context) *driver.Refusal { return nil }

// OneShots names nothing: every enforcer service stays up for the run.
func (*Driver) OneShots() []string { return nil }

// Describe reads the enforcer's image by the commit and the tree its build
// stamps, and names a development build used in place of the pin.
func (d *Driver) Describe(ctx context.Context, inspect driver.Lookup, pins driver.Pins) driver.Description {
	name, commit := pins.Value("ENFORCER_IMAGE"), pins.Value("ENFORCER_COMMIT")
	image := driver.InspectImage(ctx, inspect, name, commit, revisionLabel)
	image.TreeWant = or(pins.Value("ENFORCER_TREE"), "none in versions.env")
	if image.Missing == "" {
		tree := driver.InspectImage(ctx, inspect, name, commit, treeLabel)
		image.Tree = or(tree.Missing, tree.Label)
	}
	described := driver.Description{Images: []report.Image{image}}
	if d.development != nil {
		described.Development = d.development.describe(commit)
	}
	return described
}

// Env is the enforcer's image and its fixed address on agent-net; compose
// reads them for every run, the enforcer's service being in the topology
// whichever profile boots.
func (d *Driver) Env(run driver.Run) map[string]string {
	return map[string]string{
		"LAB_ENFORCER_REF":     d.image,
		"LAB_ENFORCER_TAG":     d.image[strings.LastIndex(d.image, ":")+1:],
		"LAB_ENFORCER_ADDRESS": run.GateAddress,
	}
}

// Admit refuses a trajectory naming another identity than the one the
// scenario's gateway part has the enforcer decide for.
func (d *Driver) Admit(spec labspec.Scenario, trajectory labspec.Trajectory) error {
	return refuseAnotherIdentity(d.config.Workspace, spec, trajectory)
}

// Prepare writes the run's signed bundle, its assembled configuration and the
// collector's certificates, and names the decision doubles' scripts.
func (d *Driver) Prepare(ctx context.Context, run driver.Run, spec labspec.Scenario) (map[string]string, error) {
	if err := d.prepareEnforcer(ctx, spec, run); err != nil {
		return nil, err
	}
	return map[string]string{
		"LAB_PDP_SCRIPT":      spec.Gateway.PDPScript,
		"LAB_APPROVER_SCRIPT": spec.Gateway.ApproverScript,
	}, nil
}

// Release removes the collector's key, which served this run alone.
func (*Driver) Release(run driver.Run) error {
	if err := dropCollectorKey(run.Dir); err != nil {
		return fmt.Errorf("removing the collector's key: %w", err)
	}
	return nil
}

// Begin starts the enforcer's part of a run once the profile has booted.
func (d *Driver) Begin(run driver.Run, spec labspec.Scenario, trajectory labspec.Trajectory) driver.GateRun {
	return &gateRun{driver: d, run: run, spec: spec, trajectory: trajectory, trail: &trailcheck.Trail{}}
}
