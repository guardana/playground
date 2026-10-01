package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/redbydesign"
)

const (
	modeCheck   = "evidence/executed-digest"
	verifyCheck = "verifier/step-2/finding/guardana.agent.mcp_server_manifest"
)

var listedTwo = []redbydesign.Entry{
	{ID: "mode-01", Checks: []string{modeCheck}, Finding: "observe completes a refused digest"},
	{ID: "verify-04", Checks: []string{verifyCheck}, Finding: "drift reported CRITICAL, catalogued HIGH"},
}

// scenarioOf builds one ran scenario from its results, the outcome taken the
// way a report takes it.
func scenarioOf(id string, results ...ranCheck) ranScenario {
	worst := assertion.Indeterminate
	if len(results) > 0 {
		worst = assertion.Pass
	}
	for _, result := range results {
		worst = assertion.Worse(worst, result.outcome)
	}
	return ranScenario{id: id, outcome: worst, results: results}
}

func passed(check string) ranCheck    { return ranCheck{id: check, outcome: assertion.Pass} }
func failed(check string) ranCheck    { return ranCheck{id: check, outcome: assertion.Fail} }
func unsettled(check string) ranCheck { return ranCheck{id: check, outcome: assertion.Indeterminate} }

func catalogueWithTwoReds() []ranScenario {
	return []ranScenario{
		scenarioOf("flow-01", passed("scenario/loads"), passed("decisions/step-1")),
		scenarioOf("mode-01", passed("scenario/loads"), passed("decisions/step-1"), failed(modeCheck)),
		scenarioOf("verify-04", passed("scenario/loads"), passed("verifier/step-1/exit-code"), failed(verifyCheck)),
	}
}

// withMode replaces mode-01 in the two-red catalogue.
func withMode(mode ranScenario) []ranScenario {
	ran := catalogueWithTwoReds()
	ran[1] = mode
	return ran
}

func TestExactlyTheListedRedsOnTheirNamedChecksPassTheRun(t *testing.T) {
	var out strings.Builder
	if err := judgeRedByDesign(catalogueWithTwoReds(), listedTwo, &out); err != nil {
		t.Fatalf("the listed reds and nothing else reported %v", err)
	}
	for _, want := range []string{"mode-01", "observe completes a refused digest", "verify-04", "1 passed"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the summary %q does not say %q", out.String(), want)
		}
	}
}

func TestEveryWayTheRedsDifferFromTheListFailsByName(t *testing.T) {
	for name, tc := range map[string]struct {
		ran  []ranScenario
		want []string
	}{
		"an unlisted red": {
			ran:  append(catalogueWithTwoReds(), scenarioOf("rule-01", failed("decisions/step-1"))),
			want: []string{"rule-01", "not listed"},
		},
		"an unlisted indeterminate": {
			ran:  append(catalogueWithTwoReds(), scenarioOf("rule-01", unsettled("decisions/step-1"))),
			want: []string{"rule-01", "not listed"},
		},
		"a listed scenario that passes": {
			ran:  withMode(scenarioOf("mode-01", passed("scenario/loads"), passed(modeCheck))),
			want: []string{"mode-01", modeCheck, "pass"},
		},
		"a listed scenario the run does not hold": {
			ran:  catalogueWithTwoReds()[:2],
			want: []string{"verify-04", "no scenario"},
		},
		"a listed scenario that established nothing": {
			ran:  withMode(ranScenario{id: "mode-01", outcome: assertion.Indeterminate}),
			want: []string{"mode-01", modeCheck, "no such result"},
		},
		"a listed scenario that did not load": {
			ran:  withMode(scenarioOf("mode-01", failed("scenario/loads"))),
			want: []string{"mode-01", "scenario/loads", modeCheck, "no such result"},
		},
		"a listed scenario failing on another check too": {
			ran:  withMode(scenarioOf("mode-01", failed("boot/victim-fs"), failed(modeCheck))),
			want: []string{"mode-01", "boot/victim-fs", "fail"},
		},
		"an indeterminate beside the named fail": {
			ran:  withMode(scenarioOf("mode-01", unsettled("evidence/trail"), failed(modeCheck))),
			want: []string{"mode-01", "evidence/trail", "indeterminate"},
		},
		"a named check that is indeterminate": {
			ran:  withMode(scenarioOf("mode-01", passed("scenario/loads"), unsettled(modeCheck))),
			want: []string{"mode-01", modeCheck, "indeterminate"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			var out strings.Builder
			err := judgeRedByDesign(tc.ran, listedTwo, &out)
			if err == nil {
				t.Fatalf("judged green; the summary was %q", out.String())
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the failure %q does not say %q", err, want)
				}
			}
		})
	}
}

func TestEveryNamedCheckMustFail(t *testing.T) {
	listed := []redbydesign.Entry{{ID: "mode-01", Checks: []string{modeCheck, "decisions/step-2"}, Finding: "f"}}
	ran := []ranScenario{scenarioOf("mode-01", failed(modeCheck), passed("decisions/step-2"))}
	err := judgeRedByDesign(ran, listed, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "decisions/step-2") {
		t.Errorf("a named check that passed reported %v", err)
	}
	ran = []ranScenario{scenarioOf("mode-01", failed(modeCheck), failed("decisions/step-2"))}
	if err := judgeRedByDesign(ran, listed, &strings.Builder{}); err != nil {
		t.Errorf("both named checks failing and nothing else reported %v", err)
	}
}

func TestEveryDifferenceIsNamedInOneRun(t *testing.T) {
	ran := []ranScenario{scenarioOf("mode-01", passed(modeCheck)), scenarioOf("rule-01", failed("decisions/step-1"))}
	err := judgeRedByDesign(ran, listedTwo, &strings.Builder{})
	if err == nil {
		t.Fatal("three differences judged green")
	}
	for _, want := range []string{"mode-01", "verify-04", "rule-01"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the failure %q does not name %s", err, want)
		}
	}
}

func TestTheListJudgesOnlyTheWholeCatalogue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "red.txt")
	if err := os.WriteFile(path, []byte("mode-01 evidence/executed-digest a finding\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	chosen, err := parse([]string{"-scenario", "mode-01", "-red-by-design", path}, &strings.Builder{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := redByDesignList(chosen); err == nil {
		t.Error("a list judged a run of one scenario")
	}
	chosen.scenario, chosen.all = "", true
	listed, err := redByDesignList(chosen)
	if err != nil || len(listed) != 1 || listed[0].ID != "mode-01" {
		t.Errorf("redByDesignList = %+v, %v", listed, err)
	}
	chosen.redByDesign = filepath.Join(t.TempDir(), "absent.txt")
	if _, err := redByDesignList(chosen); err == nil {
		t.Error("a list that does not exist judged the run")
	}
}

func TestACatalogueRunIsJudgedAgainstTheList(t *testing.T) {
	subject, _, scenario := enforcerLab(t)
	err := executeRedByDesign(context.Background(), subject, []string{scenario},
		[]redbydesign.Entry{{ID: "flow-01", Checks: []string{"decisions/step-1"}, Finding: "a finding"}}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "flow-01") {
		t.Errorf("a listed scenario that passed reported %v", err)
	}
	if err := executeRedByDesign(context.Background(), subject, []string{scenario}, nil, &strings.Builder{}); err != nil {
		t.Errorf("a green run with nothing listed reported %v", err)
	}
}

// runEach carries every result a scenario was graded on, not its outcome alone.
func TestRunEachCarriesEveryResult(t *testing.T) {
	subject, _, scenario := enforcerLab(t)
	ran := runEach(context.Background(), subject, []string{scenario}, &strings.Builder{})
	if len(ran) != 1 || len(ran[0].results) == 0 {
		t.Fatalf("runEach = %+v, want one scenario with its results", ran)
	}
	for _, result := range ran[0].results {
		if result.id == "" {
			t.Errorf("a result without its check id: %+v", ran[0].results)
		}
	}
}
