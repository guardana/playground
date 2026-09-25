package labcheck_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

const (
	failureModesPage = "docs/reference/failure-modes.md"
	useCasesPage     = "docs/reference/use-cases.md"
)

var (
	catalogRow   = regexp.MustCompile("^\\| `([A-Z]+-[0-9]{2})` \\|(.*)\\|(.*)\\|(.*)\\|$")
	modeID       = regexp.MustCompile("`([A-Z]+-[0-9]{2})`")
	shortID      = regexp.MustCompile("^[a-z]+-[0-9]{2}")
	shortInTicks = regexp.MustCompile("`([a-z]+-[0-9]{2})`")
)

// The failure-mode page and the scenarios are one record: a scenario names the
// modes it is an instance of, and the page lists, per mode, the scenarios that
// are. Either side drifting from the other fails here, so the page's coverage
// column is never a claim nobody checks.
func TestTheFailureCatalogAndTheScenariosAgree(t *testing.T) {
	listed := catalogRows(t)
	mapped := scenarioModes(t)
	for short, modes := range mapped {
		for _, mode := range modes {
			scenarios, defined := listed[mode]
			switch {
			case !defined:
				t.Errorf("%s maps to %s, which %s does not define", short, mode, failureModesPage)
			case !slices.Contains(scenarios, short):
				t.Errorf("%s maps to %s, and the row for %s does not list it", short, mode, mode)
			}
		}
	}
	for mode, scenarios := range listed {
		for _, short := range scenarios {
			if !slices.Contains(mapped[short], mode) {
				t.Errorf("the row for %s lists %s, which does not map to it", mode, short)
			}
		}
	}
}

// Every scenario of the lab's own catalogue says which failure it is an
// instance of, so a red run leads to the class of problem it belongs to.
func TestEveryLabScenarioNamesItsFailureModes(t *testing.T) {
	for _, path := range filesUnder(t, "scenarios") {
		scenario, err := labspec.LoadScenario(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(scenario.MapsTo.FailureCatalog) == 0 {
			t.Errorf("%s names no failure mode in maps_to.failure_catalog", relative(path))
		}
	}
}

// The use-case page names failure modes and scenarios by identifier; each has
// to be one the catalogue and the tree hold.
func TestTheUseCasesNameOnlyWhatExists(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot, useCasesPage))
	if err != nil {
		t.Fatal(err)
	}
	listed, mapped := catalogRows(t), scenarioModes(t)
	named := 0
	for _, match := range modeID.FindAllStringSubmatch(string(body), -1) {
		named++
		if _, defined := listed[match[1]]; !defined {
			t.Errorf("%s names %s, which %s does not define", useCasesPage, match[1], failureModesPage)
		}
	}
	for _, match := range shortInTicks.FindAllStringSubmatch(string(body), -1) {
		if _, exists := mapped[match[1]]; !exists {
			t.Errorf("%s names the scenario %s, which no scenario file is", useCasesPage, match[1])
		}
	}
	if named == 0 {
		t.Fatalf("%s names no failure mode; this guard inspected nothing", useCasesPage)
	}
}

// catalogRows reads each mode's row: its identifier and the scenarios its last
// column lists.
func catalogRows(t *testing.T) map[string][]string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repoRoot, failureModesPage))
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string][]string{}
	for _, line := range strings.Split(string(body), "\n") {
		match := catalogRow.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		if _, twice := rows[match[1]]; twice {
			t.Errorf("%s defines %s twice", failureModesPage, match[1])
		}
		scenarios := []string{}
		for _, short := range shortInTicks.FindAllStringSubmatch(match[4], -1) {
			scenarios = append(scenarios, short[1])
		}
		rows[match[1]] = scenarios
	}
	if len(rows) == 0 {
		t.Fatalf("%s defines no failure mode; this guard inspected nothing", failureModesPage)
	}
	return rows
}

// scenarioModes reads maps_to.failure_catalog from every scenario in the lab
// and in its examples, keyed by the identifier's class and number.
func scenarioModes(t *testing.T) map[string][]string {
	t.Helper()
	paths := filesUnder(t, "scenarios")
	examples, err := filepath.Glob(filepath.Join(repoRoot, "examples", "*", "scenarios", "*", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	modes := map[string][]string{}
	for _, path := range append(paths, examples...) {
		scenario, err := labspec.LoadScenario(path)
		if err != nil {
			t.Fatal(err)
		}
		short := shortID.FindString(scenario.ID)
		if short == "" {
			t.Errorf("%s: %q does not start with <class>-<nn>", relative(path), scenario.ID)
			continue
		}
		if _, twice := modes[short]; twice {
			t.Errorf("two scenarios are %s", short)
		}
		modes[short] = scenario.MapsTo.FailureCatalog
	}
	return modes
}
