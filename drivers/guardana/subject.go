package guardana

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"time"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
	"github.com/guardana/playground/runner/driver"
	"github.com/guardana/playground/runner/report"
)

// versionLabel is where the verifier's image build stamps the release it
// installed.
const versionLabel = "org.opencontainers.image.version"

// inspectTimeout bounds the docker lookup the image refusal needs.
const inspectTimeout = 30 * time.Second

// Ready refuses a run whose local verifier image was not built from the pin.
func (d *Driver) Ready(ctx context.Context) *driver.Refusal {
	if err := d.refuseImage(ctx); err != nil {
		return &driver.Refusal{Check: "verifier/image", Want: "the verifier's image built from VERIFIER_VERSION", Cause: err}
	}
	return nil
}

// refuseImage refuses the local image under the pin's tag when it was not
// built from VERIFIER_VERSION: compose never pulls it, and a tag alone says
// nothing about what was installed behind it.
func (d *Driver) refuseImage(ctx context.Context) error {
	if d.config.Inspect == nil {
		return errors.New("nothing reads the verifier's image")
	}
	ctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	want := d.config.Pins.Value("VERIFIER_VERSION")
	image := driver.InspectImage(ctx, d.config.Inspect, d.config.Pins.Value("VERIFIER_IMAGE"), want, versionLabel)
	switch {
	case image.Missing != "":
		return fmt.Errorf("%s: %s", image.Ref, image.Missing)
	case image.Label != want:
		return fmt.Errorf("%s carries %s %q, want %q", image.Ref, versionLabel, image.Label, want)
	}
	return nil
}

// OneShots names every verifier service: each runs one step and exits.
func (*Driver) OneShots() []string {
	return []string{verifierService, traceService, scanService}
}

// Describe reads the verifier's image by the release its build stamps.
func (*Driver) Describe(ctx context.Context, inspect driver.Lookup, pins driver.Pins) driver.Description {
	image := driver.InspectImage(ctx, inspect, pins.Value("VERIFIER_IMAGE"), pins.Value("VERIFIER_VERSION"), versionLabel)
	return driver.Description{Images: []report.Image{image}}
}

// Env names nothing: the verifier's services read the run's own variables.
func (*Driver) Env(driver.Run) map[string]string { return nil }

// DownProfiles names the pin step's profile: a pin step that outlived its
// deadline would otherwise be left on tool-net when the scenario's own
// profile comes down.
func (*Driver) DownProfiles() []string { return []string{verifierPinProfile} }

// Probe makes the verifier's directories before any verifier container
// starts, since compose binds directories in them and refuses to create them.
// The reach probes run with the reach directory bound, in the service the
// scenario's steps run in.
func (d *Driver) Probe(
	ctx context.Context, run driver.Run, spec labspec.Scenario, token string, reach driver.ReachOut,
) (assertion.Check, []check.StepRun) {
	dir := filepath.Join(run.Dir, verifierDir)
	made := makeDirs(dir, filepath.Join(dir, reachDir), filepath.Join(dir, reachDir, inputsDir))
	if made != nil {
		d.note("making the verifier's directories: %v", made)
	}
	service := verifierService
	if spec.IsScan() {
		service = scanService
	}
	probed := reach(ctx, run.Compose.WithEnv(map[string]string{stepVariable: reachDir}), service)
	runs := d.runVerifier(ctx, run.Compose, spec, dir, made, token)
	readRecords(runs)
	return probed, runs
}

// AgentFlags ask the agent for the trace the verifier analyses.
func (*Driver) AgentFlags(run driver.Run) []string {
	return []string{"-trace", path.Join(containerReports, run.ID, agentDir, traceFile)}
}

// Analyze has the verifier grade the agent's trace against the contract.
func (d *Driver) Analyze(ctx context.Context, run driver.Run, spec labspec.Scenario) assertion.Check {
	return d.analyzeTrace(ctx, run.Compose, spec, run.ID, run.Dir)
}
