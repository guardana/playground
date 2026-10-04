package check

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/internal/labspec"
)

// Effects compares every line each journal holds for this run against what
// the scenario says the victim or double served and refused.
//
// The comparison is exhaustive in both directions. A line whose status and tool
// the scenario does not name is a call nobody expected: a scenario that only
// checked the calls it listed would pass a run in which the agent reached one
// more tool than it was meant to, and one that only counted served calls would
// pass a run in which the gateway let through a call the victim turned away.
type Effects struct {
	Scenario labspec.Scenario
	// JournalDir is the run's journals directory, which holds one directory
	// per writer, named for it (journal.File).
	JournalDir string
}

// ID names the check in a report.
func (Effects) ID() string { return "effects" }

// Run grades one result per victim the scenario names, in victim order, and
// beside it what the victim committed wherever that was stated or recorded.
func (e Effects) Run(_ context.Context, records assertion.Records) ([]assertion.Result, error) {
	victims := slices.Sorted(maps.Keys(e.Scenario.Expect.Effects))
	results := make([]assertion.Result, 0, len(victims))
	for _, victim := range victims {
		results = append(results, e.grade(victim, records))
		if committed, graded := e.committed(victim, records); graded {
			results = append(results, committed)
		}
	}
	return results, nil
}

func (e Effects) grade(victim string, records assertion.Records) assertion.Result {
	want := e.Scenario.Expect.Effects[victim]
	result := assertion.Result{
		Check:  "effects/" + victim,
		Want:   describeEffects(want.CallsServed, want.CallsRefused),
		Source: journal.File(e.JournalDir, victim),
	}
	entries, collected := records.Journals[victim]
	if !collected {
		// A victim that never started and a victim that served nothing are the
		// two answers a denial scenario has to tell apart, and only the journal
		// tells them apart. No journal is no answer, and no answer fails.
		result.Outcome = assertion.Fail
		result.Got, result.Detail = uncollected(records, victim, "served")
		return result
	}
	// A journal is appended to and outlives one run, so a line from an earlier
	// run is a call this run never made. Counting it would let a denial that
	// held report the call it blocked.
	mine, elsewhere := servedInRun(entries, records.RunID)
	lines := tallyLines(mine)
	result.Got = describeEffects(lines.byTool(journal.Served), lines.byTool(journal.Refused))
	if unknown := lines.unknownCount(); unknown > 0 {
		result.Got += fmt.Sprintf("; %d line(s) of a status the lab does not know", unknown)
	}

	differences := slices.Concat(
		lines.compare(journal.Served, want.CallsServed),
		lines.compare(journal.Refused, want.CallsRefused),
		lines.unknownStatuses(),
	)
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

type lineKey struct {
	status journal.Status
	tool   string
}

// tally counts one run's journal lines by status and tool, and keeps the first
// line of each so a line the scenario does not name can be quoted.
type tally struct {
	count map[lineKey]int
	first map[lineKey]journal.Entry
}

func tallyLines(entries []journal.Entry) tally {
	t := tally{count: map[lineKey]int{}, first: map[lineKey]journal.Entry{}}
	for _, entry := range entries {
		key := lineKey{status: entry.Status, tool: entry.Tool}
		if t.count[key] == 0 {
			t.first[key] = entry
		}
		t.count[key]++
	}
	return t
}

func (t tally) byTool(status journal.Status) map[string]int {
	counts := map[string]int{}
	for key, n := range t.count {
		if key.status == status {
			counts[key.tool] += n
		}
	}
	return counts
}

// compare reports every disagreement for one status, in tool order, so one run
// names every unexpected call rather than the first one.
func (t tally) compare(status journal.Status, want map[string]int) []string {
	got := t.byTool(status)
	tools := slices.Sorted(maps.Keys(want))
	for tool := range got {
		if !slices.Contains(tools, tool) {
			tools = append(tools, tool)
		}
	}
	slices.Sort(tools)

	var differences []string
	for _, tool := range tools {
		expected, recorded := want[tool], got[tool]
		if expected == recorded {
			continue
		}
		if expected == 0 {
			differences = append(differences, fmt.Sprintf("%s %s %d call(s) the scenario does not name: %s",
				tool, status, recorded, quote(t.first[lineKey{status: status, tool: tool}])))
			continue
		}
		differences = append(differences, fmt.Sprintf("%s %s %d call(s), the scenario expects %d",
			tool, status, recorded, expected))
	}
	return differences
}

// unknownStatuses names every line whose status no writer in the lab emits: a
// line nobody can read a meaning into is not one a run may pass over.
func (t tally) unknownStatuses() []string {
	var keys []lineKey
	for key := range t.count {
		if !key.status.Known() {
			keys = append(keys, key)
		}
	}
	slices.SortFunc(keys, func(a, b lineKey) int {
		return cmp.Or(strings.Compare(string(a.status), string(b.status)), strings.Compare(a.tool, b.tool))
	})
	differences := make([]string, 0, len(keys))
	for _, key := range keys {
		differences = append(differences, fmt.Sprintf("%s has %d line(s) with status %q, which the lab does not know: %s",
			key.tool, t.count[key], key.status, quote(t.first[key])))
	}
	return differences
}

func (t tally) unknownCount() int {
	n := 0
	for key, count := range t.count {
		if !key.status.Known() {
			n += count
		}
	}
	return n
}

// maxQuotedDetail bounds the detail a quoted line carries into a report; a
// journal line may hold up to journal.MaxLineBytes.
const maxQuotedDetail = 512

// quote renders a journal line as the writer marshals it.
func quote(entry journal.Entry) string {
	if len(entry.Detail) > maxQuotedDetail {
		entry.Detail = entry.Detail[:maxQuotedDetail] + "…"
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Sprintf("%+v", entry)
	}
	return string(line)
}

func describeEffects(served, refused map[string]int) string {
	return describeCounts("served", served) + "; " + describeCounts("refused", refused)
}

func describeCounts(status string, counts map[string]int) string {
	if len(counts) == 0 {
		return "nothing " + status
	}
	parts := make([]string, 0, len(counts))
	for _, tool := range slices.Sorted(maps.Keys(counts)) {
		parts = append(parts, fmt.Sprintf("%s=%d", tool, counts[tool]))
	}
	return status + " " + strings.Join(parts, " ")
}

// uncollected says why a victim's journal is not in records: absent, or there
// and refused, with the reason the reader gave.
func uncollected(records assertion.Records, victim, what string) (got, detail string) {
	if reason, unread := records.Unread[victim]; unread {
		return "journal unreadable", fmt.Sprintf("the journal of %s is there and was not read, so what it %s is unknown: %s", victim, what, reason)
	}
	return "no journal", fmt.Sprintf("no journal was collected for %s, so what it %s was never read", victim, what)
}
