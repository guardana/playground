// Package driver names the roles a tool under test plays in a run and what the
// runner asks of each: a gate decides the agent's calls as they are made, a
// prober probes the victims from outside, a grader grades what the agent left
// behind. The runner drives a scenario through these alone; which tool fills
// a role is picked from the scenario.
package driver

import (
	"context"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
	"github.com/guardana/playground/runner/dockercompose"
	"github.com/guardana/playground/runner/report"
)

// Lookup runs one program and returns its trimmed output.
type Lookup func(ctx context.Context, name string, args ...string) (string, error)

// Run is one scenario's run as a subject sees it.
type Run struct {
	ID string
	// Dir is the run's directory on the host.
	Dir string
	// Compose carries the run's environment; it is nil until the run's
	// subjects are prepared.
	Compose dockercompose.Compose
}

// Refusal is a run a subject refused before anything was brought up, reported
// as the one check it fails.
type Refusal struct {
	Check string
	Want  string
	Cause error
}

// Description is what a subject says of the build a run uses, for the
// report's header.
type Description struct {
	Images []report.Image
	// Development names a build used in place of the pinned one.
	Development string
}

// Subject is one tool under test, whichever role a scenario gives it.
type Subject interface {
	// Ready refuses a run whose build of the subject is not the pinned one.
	Ready(ctx context.Context) *Refusal
	// OneShots names the subject's services that the runner runs one call at
	// a time and never brings up with the profile.
	OneShots() []string
	// Describe reads the subject's build with inspect, at pins.
	Describe(ctx context.Context, inspect Lookup, pins Pins) Description
}

// Gate decides each call the agent makes, in front of the victims.
type Gate interface {
	// Prepare writes what the gate needs before the profile boots and returns
	// the variables compose interpolates for it.
	Prepare(ctx context.Context, run Run, scenario labspec.Scenario) (map[string]string, error)
	// Release removes what Prepare left that must not outlive the run, once
	// the profile is down.
	Release(run Run) error
	// Begin starts grading the run after the profile has booted.
	Begin(run Run, scenario labspec.Scenario, trajectory labspec.Trajectory) GateRun
}

// GateRun is a gate's part of one run, in the order the runner calls it.
type GateRun interface {
	// Isolation probes that the agent reaches the gate and nothing behind it.
	Isolation(ctx context.Context) assertion.Check
	// AgentFlags point the agent at the gate.
	AgentFlags() []string
	// Chaos runs replay under the scenario's faults, and returns its check and
	// the faults' own.
	Chaos(ctx context.Context, replay func(context.Context) assertion.Check) []assertion.Check
	// AfterReplay settles the gate's record of the run before anything reads it.
	AfterReplay(ctx context.Context) []assertion.Check
	// Collect reads the gate's record of the run and returns the checks graded
	// from it, those before the victims' effects and those after.
	Collect() (before, after []assertion.Check)
	// Rows are the report's line per step, read after Collect.
	Rows() []report.StepRow
}

// Prober probes the victims from its own place on the network.
type Prober interface {
	// DownProfiles names profiles beyond the scenario's own that the prober's
	// containers can be in.
	DownProfiles() []string
	// Probe runs every step of the scenario and reads the record each left,
	// beside the check of where the prober could reach.
	Probe(ctx context.Context, run Run, scenario labspec.Scenario, token string) (assertion.Check, []check.VerifierRun)
}

// Grader grades what the agent left behind once its trajectory has run.
type Grader interface {
	// AgentFlags ask the agent for what the grader reads.
	AgentFlags(run Run) []string
	Analyze(ctx context.Context, run Run, scenario labspec.Scenario) assertion.Check
}
