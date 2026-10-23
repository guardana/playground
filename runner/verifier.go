package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	verifierreport "github.com/guardana/playground/drivers/guardana/report"
	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
	"github.com/guardana/playground/runner/dockercompose"
)

const (
	verifierService = "verifier"
	// verifierMount is where compose binds a step's directories of the run's
	// verifierDir, the only host paths the verifier sees.
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
	ctx context.Context, compose dockercompose.Compose, spec labspec.Scenario, boot assertion.Boot, runID, runDir, token string,
) assertion.Report {
	dir := filepath.Join(runDir, verifierDir)
	// Made before any verifier container starts: compose binds directories in
	// it and refuses to create them.
	made := makeDirs(dir, filepath.Join(dir, reachDir), filepath.Join(dir, reachDir, inputsDir))
	if made != nil {
		l.note("making the verifier's directories: %v", made)
	}
	reach := l.verifierReach(ctx, compose.WithEnv(map[string]string{stepVariable: reachDir}), spec, runDir)
	runs := l.runVerifier(ctx, compose, spec, dir, made, token)
	served := everyVictimServesNothing(spec, boot)
	records := l.collect(served, boot, runID, runDir, nil)
	readRecords(runs)
	checks := []assertion.Check{reach, check.Verifier{Scenario: spec, Runs: runs}, check.TokenKeptOut{RunDir: runDir, Token: token}}
	if !spec.IsScan() {
		// A scan's profile boots no service and reaches no victim, so nothing
		// came up and nothing could have been served.
		checks = append(checks,
			check.Boot{Source: filepath.Join(runDir, "boot.json")},
			check.Effects{Scenario: served, JournalDir: filepath.Join(runDir, "journals")})
	}
	if spec.AuthDouble != "" {
		checks = append(checks, check.AuthDouble{JournalDir: filepath.Join(runDir, "journals"), Persona: spec.AuthDouble})
	}
	return assertion.Run(ctx, records, checks...)
}

// everyVictimServesNothing adds every victim the profile booted to the
// effects a scenario states, as serving nothing. Every victim of the profile
// boots, a verifier reaches every one on tool-net, and a victim no step calls
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

// makeDirs makes each directory in order, readable by the verifier's uid.
func makeDirs(paths ...string) error {
	for _, path := range paths {
		if err := makeDir(path, inputDirMode); err != nil {
			return err
		}
	}
	return nil
}

// runVerifier runs every step in order. A step's pin is written into the
// step's own directory of the run, so a later step can be handed it and
// nothing outlives the run. A step whose inputs were not handed over does not
// run: what it would print could not be told from the lab's own failure.
func (l lab) runVerifier(
	ctx context.Context, compose dockercompose.Compose, spec labspec.Scenario, dir string, made error, token string,
) []check.VerifierRun {
	runs := make([]check.VerifierRun, 0, len(spec.Verifier))
	for i, step := range spec.Verifier {
		if step.Scan != nil {
			run := newScanRun(i+1, dir)
			l.runScan(ctx, compose, spec.Profile, dir, made, *step.Scan, &run)
			runs = append(runs, run)
			continue
		}
		run := newVerifierRun(i+1, *step.Probe, dir)
		l.runStep(ctx, compose, spec.Profile, dir, made, runs, token, &run)
		runs = append(runs, run)
	}
	return runs
}

func (l lab) runStep(
	ctx context.Context, compose dockercompose.Compose, profiles []string, dir string, made error, earlier []check.VerifierRun,
	token string, run *check.VerifierRun,
) {
	if made != nil {
		run.Detail = "the verifier's directory could not be made: " + made.Error()
		return
	}
	if run.Probe.Credential {
		if token == "" {
			run.Detail = "the run has no token to hand a credential step"
			return
		}
		compose = compose.WithRunEnv(mcpTokenVariable)
	}
	if err := handOver(l.workspace, dir, run.Step, run.Probe, earlier); err != nil {
		run.Detail = "the step's inputs could not be handed over: " + err.Error()
		return
	}
	l.probeOnce(ctx, compose.WithEnv(map[string]string{stepVariable: stepDirName(run.Step)}), profiles, run)
}

func newVerifierRun(number int, probe labspec.ProbeStep, dir string) check.VerifierRun {
	run := check.VerifierRun{
		Step:   number,
		Probe:  probe,
		URL:    "http://" + probe.Server + ":" + servicePort + "/mcp",
		Report: filepath.Join(dir, fmt.Sprintf("step-%d.json", number)),
	}
	if probe.WritePin {
		run.Pin = writtenPin(dir, number)
	}
	return run
}

// readRecords reads what each step left, just before it is graded.
func readRecords(runs []check.VerifierRun) {
	for i := range runs {
		readRecord(&runs[i])
	}
}

// readRecord reads the record a step is graded from: the pin a write_pin step
// approved, or the report any other step printed.
func readRecord(run *check.VerifierRun) {
	if run.Probe.WritePin {
		pin, err := verifierreport.ReadPin(run.Pin)
		if err != nil {
			run.ApprovedErr = err
			return
		}
		approved := pin.Approval()
		run.Approved = &approved
		return
	}
	ran, err := verifierreport.Read(run.Report, run.URL)
	if err != nil {
		run.RecordErr = err
		return
	}
	run.Record = &ran
}

// probeOnce runs one step and keeps both streams: standard output is the
// record a step is graded from, standard error is kept for the person reading
// a red run and never read by a check.
func (l lab) probeOnce(ctx context.Context, compose dockercompose.Compose, profiles []string, run *check.VerifierRun) {
	service, profiles := probeService(profiles, run.Probe)
	l.runOnce(ctx, compose, profiles, service, probeArgs(run.URL, run.Probe), run)
}

// runOnce runs one verifier step in service and keeps what it printed.
func (l lab) runOnce(ctx context.Context, compose dockercompose.Compose, profiles []string, service string, args []string, run *check.VerifierRun) {
	split, err := compose.RunSplit(ctx, profiles, service, "", args)
	base := strings.TrimSuffix(run.Report, ".json")
	if keepErr := keepNew(base+".stderr", split.Stderr, 0o600); keepErr != nil {
		l.note("keeping what the verifier printed on standard error: %v", keepErr)
	}
	stdout := run.Report
	if run.Probe.WritePin {
		// A pin step prints a sentence, not a report; it is kept, not graded.
		stdout = base + ".stdout"
	}
	keepErr := keepNew(stdout, split.Stdout, 0o600)
	switch {
	case err != nil:
		run.Detail = err.Error()
		return
	case keepErr != nil && !run.Probe.WritePin:
		run.Detail = "the report could not be kept: " + keepErr.Error()
		return
	case keepErr != nil:
		l.note("keeping what the verifier printed: %v", keepErr)
	}
	run.Ran, run.ExitCode = true, split.ExitCode
}

// keepNew writes what a container printed, or a file the runner hands one, to
// a path that must not exist yet: an existing file or link there is refused
// rather than written through.
func keepNew(path, body string, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode) // #nosec G304,G703 -- inside the run directory the runner made.
	if err != nil {
		return err
	}
	// The umask narrows the mode at creation; a file a container's uid reads
	// needs the mode as asked.
	err = file.Chmod(mode)
	if err == nil {
		_, err = file.WriteString(body)
	}
	return errors.Join(err, file.Close())
}
