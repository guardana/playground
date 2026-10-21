package main

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"slices"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/internal/runfile"
	"github.com/guardana/playground/runner/check"
)

// A verifier step sees one directory of the run, verifier/step-N/, and only
// through compose: in/ read-only at verifierMount/in, and for a pin step out/
// writable at verifierMount/out. What the step prints is kept beside that
// directory, where no container writes.
const (
	// verifierPinService is the verifier with its step's out/ writable; only
	// a write_pin step runs in it.
	verifierPinService = "verifier-pin"
	// verifierPinProfile holds verifierPinService alone, so no run brings it
	// up with the victims.
	verifierPinProfile = "verifier-pin"
	// stepVariable names the directory under verifier/ that compose binds.
	stepVariable = "LAB_VERIFIER_DIR"
	// reachDir is the directory the reach probes run with: no step's, and
	// empty.
	reachDir = "reach"

	inputsDir    = "in"
	outputsDir   = "out"
	profileInput = "profile.yaml"
	pinInput     = "pin.json"
	// inputDirMode and inputMode let uid 65532 read what the runner owns.
	inputDirMode fs.FileMode = 0o755
	inputMode    fs.FileMode = 0o644
	// maxInputBytes bounds a profile or a pin: a pin step's container wrote
	// the pin, and an endless one would hold the runner's memory.
	maxInputBytes = 1 << 20
)

func stepDirName(number int) string { return fmt.Sprintf("step-%d", number) }

// writtenPin is where the pin a write_pin step wrote lands on the host.
func writtenPin(dir string, step int) string {
	return filepath.Join(dir, stepDirName(step), outputsDir, pinInput)
}

// pinSource is the step whose pin this one compares against, or 0.
func pinSource(probe labspec.ProbeStep) int {
	if probe.PinOf != 0 {
		return probe.PinOf
	}
	return probe.PinFrom
}

// makeStepDirs makes a step's directory and in/, and out/ for a pin step,
// which uid 65532 writes into.
func makeStepDirs(dir string, number int, writesPin bool) error {
	step := filepath.Join(dir, stepDirName(number))
	if err := makeDir(step, inputDirMode); err != nil {
		return err
	}
	if err := makeDir(filepath.Join(step, inputsDir), inputDirMode); err != nil {
		return err
	}
	if writesPin {
		return makeShared(filepath.Join(step, outputsDir))
	}
	return nil
}

// handOver makes a step's directories and copies into in/ the profile and the
// pin it names, before its container starts. earlier holds every step that
// ran before this one, in order.
func handOver(space workspace, dir string, number int, probe labspec.ProbeStep, earlier []check.VerifierRun) error {
	if err := makeStepDirs(dir, number, probe.WritePin); err != nil {
		return err
	}
	in := filepath.Join(dir, stepDirName(number), inputsDir)
	if probe.Profile != "" {
		if err := copyInput(space.file(probe.Profile), filepath.Join(in, profileInput)); err != nil {
			return fmt.Errorf("the profile %s: %w", probe.Profile, err)
		}
	}
	if from := pinSource(probe); from != 0 {
		if err := pinFinished(earlier[from-1]); err != nil {
			return fmt.Errorf("the pin step %d wrote: %w", from, err)
		}
		if err := copyInput(writtenPin(dir, from), filepath.Join(in, pinInput)); err != nil {
			return fmt.Errorf("the pin step %d wrote: %w", from, err)
		}
	}
	return nil
}

// copyInput takes only a regular file within the bound, never a link: a pin
// came out of a container, and a link there would hand the step a host file.
func copyInput(from, to string) error {
	body, err := runfile.ReadRegular(from, maxInputBytes)
	if err != nil {
		return err
	}
	return keepNew(to, string(body), inputMode)
}

// probeArgs names each input by the path the step's container sees it at.
func probeArgs(url string, probe labspec.ProbeStep) []string {
	// A profile from a workspace can widen plugin trust; the flag wins over it.
	args := []string{"probe", "--mcp", url, "--format", "json", "--plugins", "builtins"}
	if probe.WritePin {
		args = append(args, "--write-mcp-pin", path.Join(verifierMount, outputsDir, pinInput))
	}
	if pinSource(probe) != 0 {
		args = append(args, "--mcp-pin", path.Join(verifierMount, inputsDir, pinInput))
	}
	if probe.Profile != "" {
		args = append(args, "--profile", path.Join(verifierMount, inputsDir, profileInput))
	}
	return args
}

// downProfiles names every profile a run's containers can belong to, the pin
// step's included: a pin step that outlived its deadline would otherwise be
// left on tool-net when the scenario's own profile comes down.
func downProfiles(spec labspec.Scenario) []string {
	if spec.IsVerifier() {
		return append(slices.Clone(spec.Profile), verifierPinProfile)
	}
	return spec.Profile
}

// pinFinished refuses the pin of a step that did not exit 0: a container
// killed at its deadline, or one that failed, may have left a pin it never
// finished writing, or be writing it still.
func pinFinished(source check.VerifierRun) error {
	switch {
	case !source.Ran:
		return fmt.Errorf("step %d did not run", source.Step)
	case source.ExitCode != 0:
		return fmt.Errorf("step %d exited %d", source.Step, source.ExitCode)
	}
	return nil
}

// probeService is the service a step runs in and the profiles that name it.
func probeService(profiles []string, probe labspec.ProbeStep) (string, []string) {
	if probe.WritePin {
		return verifierPinService, append(append([]string(nil), profiles...), verifierPinProfile)
	}
	return verifierService, profiles
}
