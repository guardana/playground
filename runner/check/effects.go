package check

import (
	"context"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/internal/labspec"
)

// Effects compares what each victim recorded serving against what the scenario
// says it served.
//
// The comparison is exhaustive in both directions. A tool served that the
// scenario does not name is a call nobody expected, and a scenario that only
// checked the calls it listed would pass a run in which the agent reached one
// more tool than it was meant to.
type Effects struct {
	Scenario labspec.Scenario
	// JournalDir holds one file per victim, named for the victim.
	JournalDir string
}

// ID names the check in a report.
func (Effects) ID() string { return "effects" }

// Run grades one result per victim the scenario names, in victim order.
func (e Effects) Run(_ context.Context, records assertion.Records) ([]assertion.Result, error) {
	victims := slices.Sorted(maps.Keys(e.Scenario.Expect.Effects))
	results := make([]assertion.Result, 0, len(victims))
	for _, victim := range victims {
		results = append(results, e.grade(victim, records))
	}
	return results, nil
}

func (e Effects) grade(victim string, records assertion.Records) assertion.Result {
	want := e.Scenario.Expect.Effects[victim].CallsServed
	result := assertion.Result{
		Check:  "effects/" + victim,
		Want:   describeCounts(want),
		Source: path.Join(e.JournalDir, victim+".jsonl"),
	}
	entries, collected := records.Journals[victim]
	if !collected {
		// A victim that never started and a victim that served nothing are the
		// two answers a denial scenario has to tell apart, and only the journal
		// tells them apart. No journal is no answer, and no answer fails.
		result.Outcome = assertion.Fail
		result.Got = "no journal"
		result.Detail = fmt.Sprintf(
			"no journal was collected for %s, so what it served was never read", victim)
		return result
	}
	// A journal is appended to and outlives one run, so a line from an earlier
	// run is a call this run never served. Counting it would let a denial that
	// held report the call it blocked.
	mine, elsewhere := servedInRun(entries, records.RunID)
	got := journal.CountsByTool(mine)
	result.Got = describeCounts(got)

	differences := compareCounts(want, got)
	said := slices.Clone(differences)
	if elsewhere > 0 {
		// Said either way: a reader who opens a journal holding more lines than
		// the count names has to be told why, or the count reads as a defect.
		said = append(said, fmt.Sprintf("%d line(s) recorded in another run were not counted", elsewhere))
	}
	result.Detail = strings.Join(said, "; ")
	if len(differences) > 0 {
		result.Outcome = assertion.Fail
		return result
	}
	result.Outcome = assertion.Pass
	return result
}

// compareCounts reports every disagreement, in tool order, so one run names
// every unexpected call rather than the first one.
func compareCounts(want, got map[string]int) []string {
	tools := slices.Sorted(maps.Keys(want))
	for _, tool := range slices.Sorted(maps.Keys(got)) {
		if !slices.Contains(tools, tool) {
			tools = append(tools, tool)
		}
	}
	slices.Sort(tools)

	var differences []string
	for _, tool := range tools {
		expected, served := want[tool], got[tool]
		if expected == served {
			continue
		}
		if expected == 0 {
			differences = append(differences,
				fmt.Sprintf("%s served %d call(s) the scenario does not name", tool, served))
			continue
		}
		differences = append(differences,
			fmt.Sprintf("%s served %d call(s), the scenario expects %d", tool, served, expected))
	}
	return differences
}

func describeCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return "nothing served"
	}
	parts := make([]string, 0, len(counts))
	for _, tool := range slices.Sorted(maps.Keys(counts)) {
		parts = append(parts, fmt.Sprintf("%s=%d", tool, counts[tool]))
	}
	return strings.Join(parts, " ")
}
