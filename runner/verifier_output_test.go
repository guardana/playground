package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
)

const testVerifierImage = "lab-verifier"

// pinVerifierImage pins the verifier at 0.26.1 and has the local image under
// that tag carry label as its version; any other image is answered by what
// the lab inspected before.
func pinVerifierImage(t *testing.T, subject *lab, label string) {
	t.Helper()
	writeFile(filepath.Join(subject.root, versionFile), "VERIFIER_IMAGE="+testVerifierImage+"\nVERIFIER_VERSION=0.26.1\n")
	other := subject.inspect
	subject.inspect = func(ctx context.Context, name string, args ...string) (string, error) {
		switch {
		case other == nil && !strings.HasPrefix(args[len(args)-1], testVerifierImage+":"):
			return "", errors.New("the test inspects no other image")
		case !strings.HasPrefix(args[len(args)-1], testVerifierImage+":"):
			return other(ctx, name, args...)
		case label == "":
			return "", errors.New("exit status 1: Error response from daemon: No such image: " + args[len(args)-1])
		}
		return "sha256:cc " + label, nil
	}
}

func TestAVerifierImageNotBuiltFromThePinIsRefused(t *testing.T) {
	for name, label := range map[string]string{"another release": "0.26.0", "no such image": "", "no label": "<no value>"} {
		for kind, build := range map[string]func(*testing.T) (lab, *fakeCompose, string){
			"verifier": verifierUnderTest,
			"trace":    func(t *testing.T) (lab, *fakeCompose, string) { return traceLab(t, thisRunsTrace, true) },
		} {
			t.Run(kind+"/"+name, func(t *testing.T) {
				subject, compose, scenario := build(t)
				pinVerifierImage(t, &subject, label)
				graded, err := subject.execute(context.Background(), scenario)
				if err != nil {
					t.Fatalf("execute: %v", err)
				}
				refused := results(graded)["verifier/image"]
				if refused.Outcome != assertion.Fail || graded.Outcome() == assertion.Pass {
					t.Errorf("verifier/image is %s, the run %s", refused.Outcome, graded.Outcome())
				}
				if len(compose.ran) > 0 || len(compose.broughtUp) > 0 {
					t.Errorf("the run went ahead: brought up %v, ran %v", compose.broughtUp, compose.ran)
				}
			})
		}
	}
}

// outsideFile is a file outside the run a planted link points at, and says
// whether it is still what it was.
func outsideFile(t *testing.T) (string, func() bool) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "host-file")
	writeFile(path, "untouched")
	return path, func() bool {
		body, err := os.ReadFile(path)
		return err == nil && string(body) == "untouched"
	}
}

// The probing verifier writes into the directory its step reports land in: a
// link it leaves there must not carry the runner's write out of the run, and a
// report it forged there must not be graded as the step's.
func TestAPathTheVerifierPlantedIsNotWrittenThrough(t *testing.T) {
	subject, compose, scenario := verifierUnderTest(t)
	target, untouched := outsideFile(t)
	working := compose.split
	compose.split = func(service, entrypoint string, args []string) (Split, error) {
		split, err := working(service, entrypoint, args)
		mounted := filepath.Join(compose.env["LAB_RUN_HOST_DIR"], verifierDir)
		switch {
		case slices.Contains(args, "--write-mcp-pin"):
			writeFile(filepath.Join(mounted, "step-2.json"), verifierReport)
			if err := os.Symlink(target, filepath.Join(mounted, "step-2.stderr")); err != nil {
				t.Fatal(err)
			}
		case slices.Contains(args, "--mcp-pin"):
			split.Stdout = strings.Replace(split.Stdout, "'fs.read' changed", "nothing changed", 1)
		}
		return split, err
	}
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !untouched() {
		t.Error("the runner wrote through a link the verifier planted")
	}
	if ran := results(graded)["verifier/step-2/ran"]; ran.Outcome != assertion.Indeterminate || graded.Outcome() == assertion.Pass {
		t.Errorf("a report the verifier forged was graded: step-2/ran %s, run %s", ran.Outcome, graded.Outcome())
	}
}

func TestTheTraceReportIsNotWrittenThroughAPlantedPath(t *testing.T) {
	subject, compose, scenario := traceLab(t, thisRunsTrace, true)
	pinVerifierImage(t, &subject, "0.26.1")
	target, untouched := outsideFile(t)
	working := compose.split
	compose.split = func(service, entrypoint string, args []string) (Split, error) {
		report := filepath.Join(compose.env["LAB_RUN_HOST_DIR"], verifierDir, "trace-report.json")
		if err := os.Symlink(target, report); err != nil {
			t.Fatal(err)
		}
		return working(service, entrypoint, args)
	}
	found := traceResults(t, subject, scenario)
	if !untouched() {
		t.Error("the runner wrote the trace report through a planted link")
	}
	if found["trace/ran"].Outcome == assertion.Pass || len(found) != 1 {
		t.Errorf("a trace report that could not be kept was graded: %v", found)
	}
}
