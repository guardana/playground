package claims_test

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/guardana/playground/internal/docscheck/claims"
)

func names(fsys fstest.MapFS, except ...string) []string {
	var files []string
	for name := range fsys {
		if !slices.Contains(except, name) {
			files = append(files, name)
		}
	}
	return files
}

func lines(problems []claims.Problem) []string {
	var got []string
	for _, p := range problems {
		got = append(got, p.String())
	}
	slices.Sort(got)
	return got
}

func TestEveryHeldPageCarriesItsLabels(t *testing.T) {
	fsys := fstest.MapFS{
		"README.md":                          {Data: []byte("# Lab\n\nThe lab supports two systems.\n")},
		"ROADMAP.md":                         {Data: []byte("# Roadmap\n\nIt provides a runner (implemented).\n")},
		"docs/status.md":                     {Data: []byte("# Status\n\nEnforces a pin: experimental.\n")},
		"docs/how-it-works/a-run.md":         {Data: []byte("# A run\n\nThe gateway enforces the decision.\n")},
		"docs/how-it-works/deep/verifier.md": {Data: []byte("# Verifier\n\nIt integrates with CI; planned.\n")},
		"docs/runbooks/quickstart.md":        {Data: []byte("# Quickstart\n\nThe lab supports Linux.\n")},
	}
	problems, err := claims.Unlabelled(fsys, names(fsys))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`README.md:3: capability claim without a status label: "The lab supports two systems."`,
		`docs/how-it-works/a-run.md:3: capability claim without a status label: "The gateway enforces the decision."`,
	}
	if got := lines(problems); !slices.Equal(got, want) {
		t.Errorf("problems:\n%q\nwant:\n%q", got, want)
	}
	if _, err := claims.Unlabelled(fsys, names(fsys, "ROADMAP.md")); err == nil {
		t.Error("a file list without ROADMAP.md was judged; it must be refused")
	}
}

func TestAScenarioCountLivesInStatusAndIsTrue(t *testing.T) {
	fsys := fstest.MapFS{
		"scenarios/tool/tool-01.yaml":      {Data: []byte("id: tool-01\n")},
		"scenarios/tool/tool-02.yaml":      {Data: []byte("id: tool-02\n")},
		"scenarios/chaos/chaos-01.yaml":    {Data: []byte("id: chaos-01\n")},
		"scenarios/chaos/draft.yaml":       {Data: []byte("id: draft\n")},
		"scenarios/chaos/deep/nested.yaml": {Data: []byte("id: nested\n")},
		"scenarios/red-by-design.txt":      {Data: []byte("mode-01\n")},
		"examples/e/scenarios/x/e-01.yaml": {Data: []byte("id: e-01\n")},
		"README.md": {Data: []byte("# Lab\n\nThe catalogue of 3 scenarios runs.\n" +
			"The catalogue holds 99 decided scenarios.\n")},
		"ROADMAP.md": {Data: []byte("# Roadmap\n\nThe runner, 34\nscenarios, an adopter workspace.\n")},
		"docs/status.md": {Data: []byte("# Status\n\n3 scenarios: 2 decided by the enforcer, 1 against the verifier.\n\n" +
			"Of them 4 Scenarios fail, and 1 red scenario.\n" +
			"We ran 5. Scenarios follow.\n| 7 | the | scenarios |\nThe chaos-02 scenario and flow-03 scenarios.\n")},
		"docs/lab-files.md":                  {Data: []byte("# Files\n\nTwo scenarios and scenario 3 are red.\n")},
		"CHANGELOG.md":                       {Data: []byte("# Changelog\n\n- 34 scenarios.\n")},
		"scenarios/chaos/chaos-01.notes.txt": {Data: []byte("12 scenarios\n")},
	}
	tracked := names(fsys, "scenarios/chaos/draft.yaml")
	problems, err := claims.ScenarioCount(fsys, names(fsys), tracked)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"README.md:3: states a scenario count (3); it lives in docs/status.md only",
		"README.md:4: states a scenario count (99); it lives in docs/status.md only",
		"ROADMAP.md:3: states a scenario count (34); it lives in docs/status.md only",
		"docs/status.md:5: says 1 scenarios; scenarios/*/*.yaml holds 3",
		"docs/status.md:5: says 4 scenarios; scenarios/*/*.yaml holds 3",
	}
	if got := lines(problems); !slices.Equal(got, want) {
		t.Errorf("problems:\n%q\nwant:\n%q", got, want)
	}
	if _, err := claims.ScenarioCount(fsys, names(fsys), []string{"README.md"}); err == nil {
		t.Error("a count over no scenario file was accepted; it must be refused")
	}
	if _, err := claims.ScenarioCount(fsys, names(fsys, "docs/status.md"), tracked); err == nil {
		t.Error("a file list without docs/status.md was judged; it must be refused")
	}
}

func TestTheStatusPageStatesTheTotal(t *testing.T) {
	fsys := fstest.MapFS{
		"scenarios/tool/tool-01.yaml": {Data: []byte("id: tool-01\n")},
		"scenarios/tool/tool-02.yaml": {Data: []byte("id: tool-02\n")},
		"docs/status.md":              {Data: []byte("# Status\n\nThe catalogue runs: 2 decided by the enforcer.\n")},
	}
	problems, err := claims.ScenarioCount(fsys, names(fsys), names(fsys))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"docs/status.md:1: never states the total (2) of scenarios/*/*.yaml"}
	if got := lines(problems); !slices.Equal(got, want) {
		t.Errorf("problems:\n%q\nwant:\n%q", got, want)
	}
}
