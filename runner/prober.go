package main

import (
	"context"
	"maps"
	"path/filepath"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
	"github.com/guardana/playground/runner/driver"
)

// victimPrefix starts the compose name of every victim tool server.
const victimPrefix = "victim-"

// gradeProber has the prober run each step against the victims, and grades
// the reach of its network, the records each step left, and what every victim
// served. No gate sits in the run, so no decision, trail or evidence check
// applies.
func (l lab) gradeProber(
	ctx context.Context, prober driver.Prober, run driver.Run, spec labspec.Scenario, boot assertion.Boot, token string,
) assertion.Report {
	reach, runs := prober.Probe(ctx, run, spec, token, l.reachOut(spec, run.Dir))
	served := everyVictimServesNothing(spec, boot)
	records := l.collect(served, boot, run.ID, run.Dir)
	checks := []assertion.Check{reach, check.Verifier{Scenario: spec, Runs: runs}, check.TokenKeptOut{RunDir: run.Dir, Token: token}}
	if !spec.IsScan() {
		// A scan's profile boots no service and reaches no victim, so nothing
		// came up and nothing could have been served.
		checks = append(checks,
			check.Boot{Source: filepath.Join(run.Dir, "boot.json")},
			check.Effects{Scenario: served, JournalDir: filepath.Join(run.Dir, "journals")})
	}
	if spec.AuthDouble != "" {
		checks = append(checks, check.AuthDouble{JournalDir: filepath.Join(run.Dir, "journals"), Persona: spec.AuthDouble})
	}
	return assertion.Run(ctx, records, checks...)
}

// everyVictimServesNothing adds every victim the profile booted to the
// effects a scenario states, as serving nothing. Every victim of the profile
// boots, a prober reaches every one on tool-net, and a victim no step calls
// that served a call served one nobody decided.
func everyVictimServesNothing(spec labspec.Scenario, boot assertion.Boot) labspec.Scenario {
	effects := maps.Clone(spec.Expect.Effects)
	if effects == nil {
		effects = map[string]labspec.EffectExpectation{}
	}
	for _, service := range boot.Services {
		if _, stated := effects[service.Name]; !stated && strings.HasPrefix(service.Name, victimPrefix) {
			effects[service.Name] = labspec.EffectExpectation{CallsServed: map[string]int{}}
		}
	}
	spec.Expect.Effects = effects
	return spec
}
