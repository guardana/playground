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

var (
	listedTwo = []redbydesign.Entry{
		{ID: "mode-01", Finding: "observe completes a refused digest"},
		{ID: "verify-04", Finding: "drift reported CRITICAL, catalogued HIGH"},
	}
	catalogueWithTwoReds = []ranScenario{
		{id: "flow-01", outcome: assertion.Pass},
		{id: "mode-01", outcome: assertion.Fail},
		{id: "verify-04", outcome: assertion.Fail},
	}
)

func TestExactlyTheListedRedsPassTheRun(t *testing.T) {
	var out strings.Builder
	if err := judgeRedByDesign(catalogueWithTwoReds, listedTwo, &out); err != nil {
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
			ran:  append(append([]ranScenario{}, catalogueWithTwoReds...), ranScenario{id: "rule-01", outcome: assertion.Fail}),
			want: []string{"rule-01", "not listed"},
		},
		"an unlisted indeterminate": {
			ran:  append(append([]ranScenario{}, catalogueWithTwoReds...), ranScenario{id: "rule-01", outcome: assertion.Indeterminate}),
			want: []string{"rule-01", "not listed"},
		},
		"a listed scenario that passes": {
			ran:  []ranScenario{{id: "mode-01", outcome: assertion.Pass}, {id: "verify-04", outcome: assertion.Fail}},
			want: []string{"mode-01", "passed"},
		},
		"a listed scenario the run does not hold": {
			ran:  []ranScenario{{id: "mode-01", outcome: assertion.Fail}},
			want: []string{"verify-04", "no scenario"},
		},
		"a listed scenario that established nothing": {
			ran:  []ranScenario{{id: "mode-01", outcome: assertion.Indeterminate}, {id: "verify-04", outcome: assertion.Fail}},
			want: []string{"mode-01", "indeterminate"},
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

func TestEveryDifferenceIsNamedInOneRun(t *testing.T) {
	ran := []ranScenario{{id: "mode-01", outcome: assertion.Pass}, {id: "rule-01", outcome: assertion.Fail}}
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
	if err := os.WriteFile(path, []byte("mode-01 a finding\n"), 0o600); err != nil {
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
		[]redbydesign.Entry{{ID: "flow-01", Finding: "a finding"}}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "flow-01") {
		t.Errorf("a listed scenario that passed reported %v", err)
	}
	if err := executeRedByDesign(context.Background(), subject, []string{scenario}, nil, &strings.Builder{}); err != nil {
		t.Errorf("a green run with nothing listed reported %v", err)
	}
}
