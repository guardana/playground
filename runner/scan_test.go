package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/guardana/playground/internal/assertion"
)

const scanScenarioFile = `schema_version: 1
id: scan-test
title: a pickle that imports on load is reported
profile: [scan]
verifier:
  - scan: { target: artifacts/case/defect }
expect:
  verifier:
    1:
      exit_code: 1
      gate: fail
      shortfall: []
      errors: []
      scope: [model.pkl, sub/notes.txt]
      findings: [{ rule_id: guardana.supply_chain.pickle_opcode, severity: CRITICAL, file: model.pkl, summary_contains: clab }]
`

// The scan's report as the verifier prints it for the copied target.
const scanAnswer = `{"schema_version": 18, "run": {"target": {"type": "artifact", "ref": "target"},
 "result_summary": {"rules_run": ["guardana.supply_chain.pickle_opcode"], "gate": "fail"}, "coverage": {"shortfall": []}},
 "scope": {"files": ["target/model.pkl", "target/sub/notes.txt"]},
 "findings": [{"rule_id": "guardana.supply_chain.pickle_opcode", "severity": "CRITICAL", "target_ref": "target/model.pkl",
  "evidence": {"summary": "imports clab_never_a_module.nothing"}}], "unverified": [], "waived": [], "errors": []}`

// scanUnderTest is a scan lab whose scan-verifier answers with scanAnswer when
// the target it was handed holds model.pkl, and records what it saw.
func scanUnderTest(t *testing.T) (lab, *fakeCompose, string, *[]string) {
	t.Helper()
	compose := &fakeCompose{inProfile: []string{scanService}}
	seen := &[]string{}
	compose.split = func(_, entrypoint string, args []string) (Split, error) {
		if entrypoint == "python" {
			return reachAnswer(args), nil
		}
		in := filepath.Join(compose.env["LAB_RUN_HOST_DIR"], verifierDir, "step-1", inputsDir, scanTarget)
		_ = filepath.WalkDir(in, func(path string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() {
				rel, _ := filepath.Rel(in, path)
				*seen = append(*seen, rel)
			}
			return nil
		})
		return Split{ExitCode: 1, Stdout: scanAnswer}, nil
	}
	subject, _ := labUnderTest(t, compose)
	scenario := filepath.Join(subject.root, "scenarios/scan/scan-test.yaml")
	writeFile(scenario, scanScenarioFile)
	writeFile(filepath.Join(subject.root, "artifacts/case/defect/model.pkl"), "clab_never_a_module\nnothing\n.")
	writeFile(filepath.Join(subject.root, "artifacts/case/defect/sub/notes.txt"), "a note\n")
	pinVerifierImage(t, &subject, "1.0.0rc5")
	return subject, compose, scenario, seen
}

func TestAScanStepScansACopyOfItsTargetWithNoNetwork(t *testing.T) {
	subject, compose, scenario, seen := scanUnderTest(t)
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if graded.Outcome() != assertion.Pass {
		for _, result := range graded.Results {
			t.Logf("%s: %s want=%q got=%q %s", result.Check, result.Outcome, result.Want, result.Got, result.Detail)
		}
		t.Fatalf("a scan run reported %s", graded.Outcome())
	}
	if want := []string{"model.pkl", "sub/notes.txt"}; !slices.Equal(slices.Sorted(slices.Values(*seen)), want) {
		t.Errorf("the scan saw %v, want %v", *seen, want)
	}
	if slices.ContainsFunc(compose.calls, func(call string) bool { return strings.HasPrefix(call, "up") }) {
		t.Error("a scan run brought services up; compose up with none named starts every one")
	}
	for _, call := range compose.splits {
		if call.service != scanService {
			t.Errorf("ran %s, want only %s", call.service, scanService)
		}
	}
	scan := compose.splits[len(compose.splits)-1]
	if got := strings.Join(scan.args, " "); got != "scan target --format json --plugins builtins" {
		t.Errorf("the scan ran %q", got)
	}
}

// A target holding a link, a name the verifier skips or more than the bound is
// not handed over, so the step does not run and the run does not pass.
func TestATargetThatCannotBeCopiedWholeIsNotScanned(t *testing.T) {
	for name, spoil := range map[string]func(t *testing.T, root string){
		"a link": func(t *testing.T, root string) {
			if err := os.Symlink("/etc/hosts", filepath.Join(root, "artifacts/case/defect/hosts")); err != nil {
				t.Fatal(err)
			}
		},
		"a hidden file": func(_ *testing.T, root string) {
			writeFile(filepath.Join(root, "artifacts/case/defect/.env"), "x\n")
		},
		"a directory the verifier skips": func(_ *testing.T, root string) {
			writeFile(filepath.Join(root, "artifacts/case/defect/build/model.pkl"), "x\n")
		},
		"an egg-info directory": func(_ *testing.T, root string) {
			writeFile(filepath.Join(root, "artifacts/case/defect/lab.egg-info/PKG-INFO"), "x\n")
		},
		"a file past the bound": func(_ *testing.T, root string) {
			writeFile(filepath.Join(root, "artifacts/case/defect/big.bin"), strings.Repeat("x", maxInputBytes+1))
		},
		"more files than the bound": func(_ *testing.T, root string) {
			for i := 0; i <= maxScanFiles; i++ {
				writeFile(filepath.Join(root, "artifacts/case/defect/many", strconv.Itoa(i)+".txt"), "x\n")
			}
		},
		"deeper than the bound": func(_ *testing.T, root string) {
			writeFile(filepath.Join(root, "artifacts/case/defect", strings.Repeat("d/", maxScanDepth), "deep.txt"), "x\n")
		},
		"a pipe": func(t *testing.T, root string) {
			if err := syscall.Mkfifo(filepath.Join(root, "artifacts/case/defect/pipe.txt"), 0o600); err != nil {
				t.Skipf("no fifo here: %v", err)
			}
		},
		"the target a link": func(t *testing.T, root string) {
			defect := filepath.Join(root, "artifacts/case/defect")
			moved := filepath.Join(t.TempDir(), "elsewhere")
			if err := os.Rename(defect, moved); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(moved, defect); err != nil {
				t.Fatal(err)
			}
		},
		"no target": func(t *testing.T, root string) {
			if err := os.RemoveAll(filepath.Join(root, "artifacts/case/defect")); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			subject, compose, scenario, _ := scanUnderTest(t)
			spoil(t, subject.root)
			graded, err := subject.execute(context.Background(), scenario)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if graded.Outcome() == assertion.Pass {
				t.Error("a target that was not handed over whole passed")
			}
			for _, call := range compose.splits {
				if slices.Contains(call.args, "scan") {
					t.Errorf("the scan ran: %v", call.args)
				}
			}
		})
	}
}
