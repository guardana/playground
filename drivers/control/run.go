package control

import (
	"context"
	"path/filepath"

	"github.com/guardana/playground/drivers/control/trailcheck"
	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
	"github.com/guardana/playground/runner/driver"
	"github.com/guardana/playground/runner/report"
)

// gateRun holds the enforcer's trail, which its checks share and Collect
// fills once the plane has drained.
type gateRun struct {
	driver     *Driver
	run        driver.Run
	spec       labspec.Scenario
	trajectory labspec.Trajectory
	trail      *trailcheck.Trail
}

func (r *gateRun) Endpoint() string { return enforcerService + ":" + servicePort }

// Sealed probes the plane's own services, the proxies the run routes through,
// and the enforcer's agent listener from the networks it shares with the
// victims.
func (r *gateRun) Sealed(ctx context.Context, reach driver.Reach) []check.Probe {
	proxies, unnamed := r.driver.sealedProxies(r.spec)
	var sealed []check.Probe
	for _, target := range append(sealedFromAgent(r.spec), proxies...) {
		sealed = append(sealed, reach(ctx, target))
	}
	if unnamed != nil {
		sealed = append(sealed, check.Probe{Target: proxyService, Detail: unnamed.Error()})
	}
	return append(sealed, listenerProbes(ctx, r.run.Compose, r.spec, r.trajectory, r.run.GateAddress)...)
}

func (r *gateRun) AgentFlags() []string {
	return []string{"-gateway", "http://" + r.Endpoint() + "/mcp", "-namespace", r.driver.namespace}
}

// Chaos applies the scenario's faults, replays the trajectory and lifts the
// faults again, all before anything drains the trail.
func (r *gateRun) Chaos(ctx context.Context, replay func(context.Context) assertion.Check) []assertion.Check {
	if len(r.spec.Chaos) == 0 {
		return []assertion.Check{replay(ctx)}
	}
	applied := r.driver.applyChaos(ctx, r.run.Compose, r.spec)
	replayed := replay(ctx)
	graded := r.driver.liftChaos(ctx, r.run.Compose, r.spec, applied, r.run.Dir)
	graded.Scenario, graded.Trajectory, graded.Trail = r.spec, r.trajectory, r.trail
	return []assertion.Check{replayed, graded}
}

func (r *gateRun) AfterReplay(ctx context.Context) []assertion.Check {
	checks := []assertion.Check{
		r.driver.drainPlane(ctx, r.run.Compose, r.spec.Profile, r.driver.pin, r.run.Dir),
		trailcheck.TrailClaims{Scenario: r.spec, EvidenceFile: r.evidenceFile(), Trail: r.trail},
	}
	if r.spec.Expect.Health != nil {
		checks = append(checks, trailcheck.Health{Expect: *r.spec.Expect.Health, Source: filepath.Join(r.run.Dir, healthRecord)})
	}
	return checks
}

func (r *gateRun) Collect() (before, after []assertion.Check) {
	*r.trail = r.driver.collectTrail(r.run.Dir)
	evidenceFile := r.evidenceFile()
	before = []assertion.Check{
		trailcheck.Decisions{Scenario: r.spec, Trajectory: r.trajectory, EvidenceFile: evidenceFile, Trail: r.trail},
		trailcheck.Trails{Scenario: r.spec, Trajectory: r.trajectory, EvidenceFile: evidenceFile, Trail: r.trail},
	}
	after = []assertion.Check{trailcheck.Evidence{
		Scenario: r.spec, EvidenceFile: evidenceFile, Trail: r.trail,
		// The trail is read from the run's directory, which the runner created
		// for this run and refuses when it already exists.
		FreshTrail: true,
	}}
	return before, after
}

func (r *gateRun) Rows() []report.StepRow {
	return trailcheck.DecisionRows(r.spec, r.trajectory, r.trail.Events, r.evidenceFile())
}

func (r *gateRun) evidenceFile() string { return filepath.Join(r.run.Dir, "evidence.jsonl") }
