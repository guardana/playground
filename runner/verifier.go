package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

const (
	verifierService = "verifier"
	// verifierMount is where compose mounts the run's verifierDir, the only
	// place the verifier can write a pin or read one back.
	verifierMount = "/lab-run"
	verifierDir   = "verifier"
	// victimPrefix starts the compose name of every victim tool server.
	victimPrefix = "victim-"
)

// gradeVerifier runs each verifier step against the victims and grades the
// reach of the verifier's network, the records each step left, and what every
// victim served. There is no enforcer in the run, so no decision, trail or
// evidence check applies.
func (l lab) gradeVerifier(
	ctx context.Context, compose Compose, spec labspec.Scenario, boot assertion.Boot, runID, runDir string,
) assertion.Report {
	dir := filepath.Join(runDir, verifierDir)
	// Made before any verifier container starts: compose mounts it and refuses
	// to create it.
	shared := makeShared(dir)
	reach := l.verifierReach(ctx, compose, spec, runDir)
	runs := l.runVerifier(ctx, compose, spec, dir, shared)
	served := everyVictimServesNothing(spec, boot)
	records, _ := l.collect(served, boot, runID, runDir)
	return assertion.Run(ctx, records,
		check.Boot{Source: filepath.Join(runDir, "boot.json")},
		reach,
		check.Verifier{Scenario: spec, Runs: runs},
		check.Effects{Scenario: served, JournalDir: filepath.Join(runDir, "journals")},
	)
}

// everyVictimServesNothing adds every victim the profile booted to the
// effects a verifier scenario states, as serving nothing. The verifier reaches
// every victim on tool-net, not only the ones it probes, and it documents that
// it calls no tool on any of them.
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

// runVerifier runs every step in order. A step's pin is written into the
// run's verifier directory, so a later step can compare against it and nothing
// outlives the run.
func (l lab) runVerifier(
	ctx context.Context, compose Compose, spec labspec.Scenario, dir string, shared error,
) []check.VerifierRun {
	runs := make([]check.VerifierRun, 0, len(spec.Verifier))
	for i, step := range spec.Verifier {
		run := newVerifierRun(i+1, *step.Probe, dir)
		if shared != nil {
			run.Detail = "the directory the verifier writes into could not be made: " + shared.Error()
		} else {
			l.probeOnce(ctx, compose, spec.Profile, &run)
		}
		runs = append(runs, run)
	}
	return runs
}

func newVerifierRun(number int, probe labspec.ProbeStep, dir string) check.VerifierRun {
	run := check.VerifierRun{
		Step:   number,
		Probe:  probe,
		URL:    "http://" + probe.Server + ":" + servicePort + "/mcp",
		Report: filepath.Join(dir, fmt.Sprintf("step-%d.json", number)),
	}
	if probe.WritePin {
		run.Pin = filepath.Join(dir, pinName(number))
	}
	return run
}

// probeOnce runs one step and keeps both streams: standard output is the
// record a step is graded from, standard error is kept for the person reading
// a red run and never read by a check.
func (l lab) probeOnce(ctx context.Context, compose Compose, profiles []string, run *check.VerifierRun) {
	args := []string{"probe", "--mcp", run.URL, "--format", "json"}
	if run.Probe.WritePin {
		args = append(args, "--write-mcp-pin", containerPin(run.Step))
	}
	if run.Probe.PinFrom != 0 {
		args = append(args, "--mcp-pin", containerPin(run.Probe.PinFrom))
	}
	split, err := compose.RunSplit(ctx, profiles, verifierService, "", args)
	stdout := run.Report
	if run.Probe.WritePin {
		// A pin step prints a sentence, not a report; it is kept, not graded.
		stdout = strings.TrimSuffix(run.Report, ".json") + ".stdout"
	}
	for file, body := range map[string]string{stdout: split.Stdout, strings.TrimSuffix(run.Report, ".json") + ".stderr": split.Stderr} {
		// #nosec G703 -- the path is inside the run directory the runner made.
		if writeErr := os.WriteFile(file, []byte(body), 0o600); writeErr != nil {
			l.note("writing what the verifier printed: %v", writeErr)
		}
	}
	if err != nil {
		run.Detail = err.Error()
		return
	}
	run.Ran, run.ExitCode = true, split.ExitCode
}

func pinName(step int) string { return fmt.Sprintf("pin-%d.json", step) }

func containerPin(step int) string { return path.Join(verifierMount, pinName(step)) }
