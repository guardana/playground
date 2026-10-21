package main

import (
	"context"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/guardana/playground/internal/assertion"
)

const (
	profiledScenarioFile = `schema_version: 1
id: verify-handover
title: a pin the profile leaves out leaves the run indeterminate
profile: [verifier]
verifier:
  - probe: { server: victim-fs, write_pin: true }
  - probe: { server: victim-fs, pin_from: 1, profile: config/verifier/profiles/strict.yaml }
expect:
  verifier:
    1: { exit_code: 0 }
    2:
      exit_code: 2
      gate: indeterminate
      shortfall: [{ kind: demanded_check, name: guardana.agent.mcp_server_manifest }]
      errors: []
  effects:
    victim-fs: { calls_served: {} }
`
	profileBody = "fail_on_inconclusive: true\n"
	// indeterminateReport is what the verifier prints when the pin it was
	// handed is one its profile does not check.
	indeterminateReport = `{"schema_version": 18, "run": {"target": {"ref": "http://victim-fs:8080/mcp"},
 "result_summary": {"gate": "indeterminate", "rules_run": ["guardana.mcp.cache_scope"]},
 "coverage": {"shortfall": [{"kind": "demanded_check", "name": "guardana.agent.mcp_server_manifest", "detail": "excluded"}]}},
 "findings": [], "unverified": [], "errors": []}`
)

// profiledUnderTest is the verifier lab running a scenario whose second step
// compares against the first step's pin under a profile, and is answered with
// the indeterminate report that scenario expects.
func profiledUnderTest(t *testing.T) (lab, *fakeCompose, string) {
	t.Helper()
	subject, compose, _ := verifierUnderTest(t)
	scenario := filepath.Join(subject.root, "scenarios/verify/verify-handover.yaml")
	writeFile(scenario, profiledScenarioFile)
	writeFile(filepath.Join(subject.root, "config/verifier/profiles/strict.yaml"), profileBody)
	working := compose.split
	compose.split = func(service, entrypoint string, args []string) (Split, error) {
		split, err := working(service, entrypoint, args)
		if err == nil && slices.Contains(args, "--mcp-pin") && split.ExitCode != 3 {
			split.ExitCode, split.Stdout = 2, indeterminateReport
		}
		return split, err
	}
	return subject, compose, scenario
}

// An indeterminate run is the verifier's answer only when the step was handed
// what it names: a step whose profile or pin did not reach it does not run,
// whatever a container would have printed.
func TestAStepWhoseInputsWereNotHandedOverDoesNotRun(t *testing.T) {
	for name, spoil := range handoverSpoilers() {
		t.Run(name, func(t *testing.T) {
			subject, compose, scenario := profiledUnderTest(t)
			spoil(t, subject, compose)
			answered := compose.split
			compose.split = func(service, entrypoint string, args []string) (Split, error) {
				if slices.Contains(args, "--mcp-pin") {
					return Split{ExitCode: 2, Stdout: indeterminateReport}, nil
				}
				return answered(service, entrypoint, args)
			}
			graded, err := subject.execute(context.Background(), scenario)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			wantNotRun(t, graded, compose)
		})
	}
}

// handoverSpoilers each break one input of step 2 before it is handed over.
func handoverSpoilers() map[string]func(t *testing.T, subject lab, compose *fakeCompose) {
	return map[string]func(t *testing.T, subject lab, compose *fakeCompose){
		"the profile a link": func(t *testing.T, subject lab, _ *fakeCompose) {
			profiles := filepath.Join(subject.root, "config/verifier/profiles")
			writeFile(filepath.Join(profiles, "other.yaml"), profileBody)
			_ = os.Remove(filepath.Join(profiles, "strict.yaml"))
			if err := os.Symlink("other.yaml", filepath.Join(profiles, "strict.yaml")); err != nil {
				t.Fatal(err)
			}
		},
		"the profile gone after the scenario was read": func(_ *testing.T, subject lab, compose *fakeCompose) {
			working := compose.split
			compose.split = func(service, entrypoint string, args []string) (Split, error) {
				_ = os.Remove(filepath.Join(subject.root, "config/verifier/profiles/strict.yaml"))
				return working(service, entrypoint, args)
			}
		},
		"a pin left by a step that failed": func(_ *testing.T, _ lab, compose *fakeCompose) {
			working := compose.split
			compose.split = func(service, entrypoint string, args []string) (Split, error) {
				split, err := working(service, entrypoint, args)
				if slices.Contains(args, "--write-mcp-pin") {
					split.ExitCode = 1
				}
				return split, err
			}
		},
		// A valid pin padded past the bound: only the bound refuses it.
		"a pin past the bound": func(_ *testing.T, _ lab, compose *fakeCompose) {
			working := compose.split
			compose.split = func(service, entrypoint string, args []string) (Split, error) {
				split, err := working(service, entrypoint, args)
				if slices.Contains(args, "--write-mcp-pin") {
					pin := writtenPin(filepath.Join(compose.env["LAB_RUN_HOST_DIR"], verifierDir), 1)
					writeFile(pin, verifierPin+strings.Repeat(" ", 1<<20))
				}
				return split, err
			}
		},
		"no pin written": func(_ *testing.T, _ lab, compose *fakeCompose) {
			working := compose.split
			compose.split = func(service, entrypoint string, args []string) (Split, error) {
				if slices.Contains(args, "--write-mcp-pin") {
					return Split{Stdout: "Wrote 1 approved tool description(s)\n"}, nil
				}
				return working(service, entrypoint, args)
			}
		},
	}
}

// wantNotRun holds step 2 to not having run, and the run to not passing.
func wantNotRun(t *testing.T, graded assertion.Report, compose *fakeCompose) {
	t.Helper()
	// A step that ran reports no ran result at all, so its absence is the
	// step having run.
	ran, recorded := results(graded)["verifier/step-2/ran"]
	if !recorded || ran.Outcome == assertion.Pass || graded.Outcome() == assertion.Pass {
		t.Errorf("a step whose handover failed: step-2/ran %s, run %s", ran.Outcome, graded.Outcome())
	}
	if !strings.Contains(ran.Detail, "handed over") {
		t.Errorf("step-2/ran says %q, want why the step did not run", ran.Detail)
	}
	for _, call := range compose.splits {
		if slices.Contains(call.args, "--mcp-pin") {
			t.Errorf("the step ran: %v", call.args)
		}
	}
}

// inputSeen is what a step's input directory held when its container started.
type inputSeen struct {
	dir   fs.FileMode
	files map[string]string
}

func TestAStepsInputsAreWorldReadableFilesBeforeItRuns(t *testing.T) {
	previous := syscall.Umask(0o077)
	defer syscall.Umask(previous)
	subject, compose, scenario := profiledUnderTest(t)
	var seen *inputSeen
	working := compose.split
	compose.split = func(service, entrypoint string, args []string) (Split, error) {
		if slices.Contains(args, "--mcp-pin") {
			seen = readInputs(t, filepath.Join(compose.env["LAB_RUN_HOST_DIR"], "verifier", "step-2", "in"))
		}
		return working(service, entrypoint, args)
	}
	if _, err := subject.execute(context.Background(), scenario); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if seen == nil {
		t.Fatal("the step was never run")
	}
	if seen.dir != fs.ModeDir|0o755 {
		t.Errorf("step-2/in is %v, want drwxr-xr-x", seen.dir)
	}
	want := map[string]string{
		"pin.json":     "-rw-r--r-- " + verifierPin,
		"profile.yaml": "-rw-r--r-- " + profileBody,
	}
	for name, got := range seen.files {
		if got != want[name] {
			t.Errorf("step-2/in/%s is %q, want %q", name, got, want[name])
		}
	}
	if len(seen.files) != len(want) {
		t.Errorf("step-2/in holds %v, want %v", slices.Sorted(maps.Keys(seen.files)), slices.Sorted(maps.Keys(want)))
	}
}

func readInputs(t *testing.T, dir string) *inputSeen {
	t.Helper()
	info, err := os.Lstat(dir)
	if err != nil {
		t.Errorf("no input directory: %v", err)
		return &inputSeen{}
	}
	seen := &inputSeen{dir: info.Mode(), files: map[string]string{}}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		file, err := os.Lstat(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		body, _ := os.ReadFile(filepath.Join(dir, entry.Name()))
		seen.files[entry.Name()] = file.Mode().String() + " " + string(body)
	}
	return seen
}

// Each step names its own directory to compose, and the arguments name the
// files in it; only the pin step runs the service that binds out/ writable.
func TestEachStepRunsWithItsOwnDirectoryAndOnlyThePinStepWrites(t *testing.T) {
	subject, compose, scenario := profiledUnderTest(t)
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got []string
	for _, call := range compose.splits {
		if call.args[0] == "probe" {
			got = append(got, call.service+" "+call.env["LAB_VERIFIER_DIR"]+" "+strings.Join(call.profiles, ",")+" "+strings.Join(call.args, " "))
		}
	}
	want := []string{
		"verifier-pin step-1 verifier,verifier-pin probe --mcp http://victim-fs:8080/mcp --format json --plugins builtins --write-mcp-pin /lab-run/out/pin.json",
		"verifier step-2 verifier probe --mcp http://victim-fs:8080/mcp --format json --plugins builtins --mcp-pin /lab-run/in/pin.json --profile /lab-run/in/profile.yaml",
	}
	if !slices.Equal(got, want) {
		t.Errorf("steps ran as\n%q\nwant\n%q", got, want)
	}
	verifier := filepath.Join(subject.reports, graded.RunID, "verifier")
	if info, err := os.Lstat(filepath.Join(verifier, "step-1", "out")); err != nil || info.Mode() != fs.ModeDir|fs.ModeSticky|0o777 {
		t.Errorf("the pin step's out/ is %v, %v; want a sticky directory uid 65532 can write", info, err)
	}
	if _, err := os.Lstat(filepath.Join(verifier, "step-2", "out")); err == nil {
		t.Error("a step that writes no pin was given an out/ directory")
	}
	for _, kept := range []string{"step-1.stdout", "step-2.json", "step-2.stderr"} {
		if _, err := os.Lstat(filepath.Join(verifier, kept)); err != nil {
			t.Errorf("%s was not kept outside the step directories: %v", kept, err)
		}
	}
}

// A variable added for one call keeps every variable the run was given: a
// step that lost COMPOSE_PROJECT_NAME would run in another project.
func TestAVariableAddedForOneCallKeepsTheRunsOwn(t *testing.T) {
	run := dockerCompose{}.WithEnv(map[string]string{"COMPOSE_PROJECT_NAME": "lab-run1", "LAB_RUN_HOST_DIR": "/r"})
	step := run.WithEnv(map[string]string{"LAB_VERIFIER_DIR": "step-1"}).(dockerCompose)
	got := map[string]string{}
	for _, entry := range step.env {
		name, value, _ := strings.Cut(entry, "=")
		got[name] = value
	}
	for name, want := range map[string]string{"COMPOSE_PROJECT_NAME": "lab-run1", "LAB_RUN_HOST_DIR": "/r", "LAB_VERIFIER_DIR": "step-1"} {
		if got[name] != want {
			t.Errorf("%s = %q, want %q", name, got[name], want)
		}
	}
	for _, entry := range run.(dockerCompose).env {
		if strings.HasPrefix(entry, "LAB_VERIFIER_DIR=") {
			t.Error("a variable added for one call reached the run's own compose")
		}
	}
}

// A pin step's container belongs to a profile the scenario does not name, so
// the teardown names it too: one left running would sit on tool-net beside the
// next run's victims.
func TestAVerifierRunTakesDownThePinStepsService(t *testing.T) {
	subject, compose, scenario := verifierUnderTest(t)
	if _, err := subject.execute(context.Background(), scenario); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !slices.Contains(compose.downProfiles, verifierPinProfile) || !slices.Contains(compose.downProfiles, "verifier") {
		t.Errorf("down was called for %v, want verifier and %s", compose.downProfiles, verifierPinProfile)
	}
}
