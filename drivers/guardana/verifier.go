package guardana

import (
	"context"
	"fmt"
	"path/filepath"

	verifierreport "github.com/guardana/playground/drivers/guardana/report"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/internal/runfile"
	"github.com/guardana/playground/runner/check"
	"github.com/guardana/playground/runner/dockercompose"
)

// makeDirs makes each directory in order, readable by the verifier's uid.
func makeDirs(paths ...string) error {
	for _, path := range paths {
		if err := runfile.MakeDir(path, inputDirMode); err != nil {
			return err
		}
	}
	return nil
}

// reportRun is a step graded from the JSON report it prints about target.
func reportRun(number int, target, report string) check.StepRun {
	return check.StepRun{
		Step: number, Target: target, Kind: check.ReportRecord, Want: "a JSON report about " + target, Source: report,
	}
}

// pinRun is a step graded from the pin it writes approving target.
func pinRun(number int, target, pin string) check.StepRun {
	return check.StepRun{
		Step: number, Target: target, Kind: check.PinRecord, Want: "a pin approving " + target, Source: pin,
	}
}

// runVerifier runs every step in order. A step's pin is written into the
// step's own directory of the run, so a later step can be handed it and
// nothing outlives the run. A step whose inputs were not handed over does not
// run: what it would print could not be told from the lab's own failure.
func (d *Driver) runVerifier(
	ctx context.Context, compose dockercompose.Compose, spec labspec.Scenario, dir string, made error, token string,
) []check.StepRun {
	runs := make([]check.StepRun, 0, len(spec.Verifier))
	for i, step := range spec.Verifier {
		if step.Scan != nil {
			run := newScanRun(i+1, dir)
			d.runScan(ctx, compose, spec.Profile, dir, made, *step.Scan, &run)
			runs = append(runs, run)
			continue
		}
		run := newVerifierRun(i+1, *step.Probe, dir)
		d.runStep(ctx, compose, spec.Profile, dir, made, runs, token, *step.Probe, &run)
		runs = append(runs, run)
	}
	return runs
}

func (d *Driver) runStep(
	ctx context.Context, compose dockercompose.Compose, profiles []string, dir string, made error, earlier []check.StepRun,
	token string, probe labspec.ProbeStep, run *check.StepRun,
) {
	if made != nil {
		run.Detail = "the verifier's directory could not be made: " + made.Error()
		return
	}
	if probe.Credential {
		if token == "" {
			run.Detail = "the run has no token to hand a credential step"
			return
		}
		compose = compose.WithRunEnv(labspec.MCPTokenVariable)
	}
	if err := d.handOver(dir, run.Step, probe, earlier); err != nil {
		run.Detail = "the step's inputs could not be handed over: " + err.Error()
		return
	}
	step := compose.WithEnv(map[string]string{stepVariable: stepDirName(run.Step)})
	service, profiles := probeService(profiles, probe)
	d.runOnce(ctx, step, profiles, service, probeArgs(run.Target, probe), dir, run)
}

func newVerifierRun(number int, probe labspec.ProbeStep, dir string) check.StepRun {
	url := "http://" + probe.Server + ":" + servicePort + "/mcp"
	if probe.WritePin {
		return pinRun(number, url, writtenPin(dir, number))
	}
	return reportRun(number, url, filepath.Join(dir, fmt.Sprintf("step-%d.json", number)))
}

// readRecords reads what each step left, once every step has run.
func readRecords(runs []check.StepRun) {
	for i := range runs {
		readRecord(&runs[i])
	}
}

// readRecord reads the record a step is graded from: the pin a pin step
// approved, or the report any other step printed.
func readRecord(run *check.StepRun) {
	if run.Kind == check.PinRecord {
		pin, err := verifierreport.ReadPin(run.Source)
		if err != nil {
			run.RecordErr = err
			return
		}
		approved := pin.Approval()
		run.Approved = &approved
		return
	}
	ran, err := verifierreport.Read(run.Source, run.Target)
	if err != nil {
		run.RecordErr = err
		return
	}
	run.Record = &ran
}

// runOnce runs one verifier step in service and keeps both streams beside the
// step's directory: standard output is the record a step is graded from,
// standard error is kept for the person reading a red run and never read by a
// check.
func (d *Driver) runOnce(
	ctx context.Context, compose dockercompose.Compose, profiles []string, service string, args []string, dir string,
	run *check.StepRun,
) {
	split, err := compose.RunSplit(ctx, profiles, service, "", args)
	base := filepath.Join(dir, stepDirName(run.Step))
	if keepErr := runfile.KeepNew(base+".stderr", split.Stderr, 0o600); keepErr != nil {
		d.note("keeping what the verifier printed on standard error: %v", keepErr)
	}
	pin := run.Kind == check.PinRecord
	stdout := run.Source
	if pin {
		// A pin step prints a sentence, not a report; it is kept, not graded.
		stdout = base + ".stdout"
	}
	keepErr := runfile.KeepNew(stdout, split.Stdout, 0o600)
	switch {
	case err != nil:
		run.Detail = err.Error()
		return
	case keepErr != nil && !pin:
		run.Detail = "the report could not be kept: " + keepErr.Error()
		return
	case keepErr != nil:
		d.note("keeping what the verifier printed: %v", keepErr)
	}
	run.Ran, run.ExitCode = true, split.ExitCode
}
