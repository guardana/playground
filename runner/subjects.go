package main

import (
	"slices"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/driver"
)

// subjects is every tool the lab drives, in the order the report describes
// them, whichever a scenario picks.
func (l lab) subjects() []driver.Subject {
	return []driver.Subject{l.control, newGuardana(l)}
}

// picked is what drives one scenario: each role it fills, nil where it fills
// none, and the subjects behind them, each once.
type picked struct {
	subjects []driver.Subject
	gate     driver.Gate
	prober   driver.Prober
	grader   driver.Grader
}

// pick reads the roles from a scenario's keys: a gateway is decided by the
// enforcer, and verifier steps and a trace are run by the verifier.
func (l lab) pick(spec labspec.Scenario) picked {
	var chosen picked
	if spec.Gateway != nil {
		chosen.gate = l.control
		chosen.subjects = append(chosen.subjects, l.control)
	}
	if !spec.IsVerifier() && spec.Trace == nil {
		return chosen
	}
	guardana := newGuardana(l)
	chosen.subjects = append(chosen.subjects, guardana)
	if spec.IsVerifier() {
		chosen.prober = guardana
	}
	if spec.Trace != nil {
		chosen.grader = guardana
	}
	return chosen
}

// downProfiles names every profile a run's containers can belong to.
func (p picked) downProfiles(spec labspec.Scenario) []string {
	if p.prober == nil {
		return spec.Profile
	}
	return append(slices.Clone(spec.Profile), p.prober.DownProfiles()...)
}

// oneShots names every service the runner runs one call at a time: the agent
// and each subject's own, whether or not the scenario picks it.
func (l lab) oneShots() []string {
	shots := []string{agentService}
	for _, subject := range l.subjects() {
		shots = append(shots, subject.OneShots()...)
	}
	return shots
}

// admit has the scenario's gate refuse a trajectory it could not decide as
// written; a scenario with no gate has nothing to refuse.
func (l lab) admit(spec labspec.Scenario, trajectory labspec.Trajectory) error {
	if gate := l.pick(spec).gate; gate != nil {
		return gate.Admit(spec, trajectory)
	}
	return nil
}
