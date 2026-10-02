package check

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/journal"
)

// committed grades the effects a victim's served lines carry against
// expect.effects.<victim>.committed, in journal order, member for member. It
// reports false when there is nothing to grade: nothing stated and, in a
// journal that was read, no effect on any line of this run.
func (e Effects) committed(victim string, records assertion.Records) (assertion.Result, bool) {
	want := e.Scenario.Expect.Effects[victim].Committed
	result := assertion.Result{
		Check:  "effects/" + victim + "/committed",
		Want:   describeCommitted(want),
		Source: path.Join(e.JournalDir, victim+".jsonl"),
	}
	entries, collected := records.Journals[victim]
	if !collected {
		if len(want) == 0 {
			return result, false
		}
		result.Outcome = assertion.Fail
		result.Got = "no journal"
		result.Detail = fmt.Sprintf("no journal was collected for %s, so what it committed was never read", victim)
		return result, true
	}
	mine, elsewhere := servedInRun(entries, records.RunID)
	got, stray := committedLines(mine)
	if len(want) == 0 && !holdsEffect(got) && len(stray) == 0 {
		return result, false
	}
	result.Got = describeCommitted(shown(got, want))

	differences := slices.Concat(compareCommitted(want, got), stray)
	said := slices.Clone(differences)
	if elsewhere > 0 {
		said = append(said, fmt.Sprintf("%d line(s) recorded in another run were not graded", elsewhere))
	}
	result.Detail = strings.Join(said, "; ")
	result.Outcome = assertion.Pass
	if len(differences) > 0 {
		result.Outcome = assertion.Fail
	}
	return result, true
}

// committedLines lists, per tool, the effect on each served line in journal
// order, nil where a served line carries none. An effect on any other line is
// one no writer in the lab emits, and is named.
func committedLines(entries []journal.Entry) (map[string][]journal.Effect, []string) {
	got := map[string][]journal.Effect{}
	var stray []string
	for _, entry := range entries {
		switch {
		case entry.Status == journal.Served:
			got[entry.Tool] = append(got[entry.Tool], entry.Effect)
		case entry.Effect != nil:
			stray = append(stray, fmt.Sprintf("%s: a %q line carries an effect: %s", entry.Tool, entry.Status, quote(entry)))
		}
	}
	return got, stray
}

func holdsEffect(got map[string][]journal.Effect) bool {
	for _, effects := range got {
		if slices.ContainsFunc(effects, func(effect journal.Effect) bool { return effect != nil }) {
			return true
		}
	}
	return false
}

// compareCommitted names every disagreement, in tool order: a named tool whose
// served lines differ in number, lack an effect or state another; a tool the
// scenario does not name whose served lines carry one.
func compareCommitted(want, got map[string][]journal.Effect) []string {
	tools := slices.Sorted(maps.Keys(want))
	for tool := range got {
		if !slices.Contains(tools, tool) {
			tools = append(tools, tool)
		}
	}
	slices.Sort(tools)

	var differences []string
	for _, tool := range tools {
		expected, stated := want[tool]
		if !stated {
			for i, effect := range got[tool] {
				if effect != nil {
					differences = append(differences, fmt.Sprintf("%s #%d committed %s, which the scenario does not name",
						tool, i+1, effect))
				}
			}
			continue
		}
		recorded := got[tool]
		if len(recorded) != len(expected) {
			differences = append(differences, fmt.Sprintf("%s served %d call(s), the scenario states %d effect(s)",
				tool, len(recorded), len(expected)))
		}
		for i := range min(len(recorded), len(expected)) {
			switch {
			case recorded[i] == nil:
				differences = append(differences, fmt.Sprintf("%s #%d was served with no effect recorded", tool, i+1))
			case !recorded[i].Equal(expected[i]):
				differences = append(differences, fmt.Sprintf("%s #%d committed %s, the scenario expects %s",
					tool, i+1, recorded[i], expected[i]))
			}
		}
	}
	return differences
}

// shown keeps the tools a reader of the result needs: those the scenario names
// and those whose served lines carry an effect.
func shown(got, want map[string][]journal.Effect) map[string][]journal.Effect {
	kept := map[string][]journal.Effect{}
	for tool, effects := range got {
		_, stated := want[tool]
		if stated || slices.ContainsFunc(effects, func(effect journal.Effect) bool { return effect != nil }) {
			kept[tool] = effects
		}
	}
	return kept
}

func describeCommitted(effects map[string][]journal.Effect) string {
	if len(effects) == 0 {
		return "nothing committed"
	}
	parts := make([]string, 0, len(effects))
	for _, tool := range slices.Sorted(maps.Keys(effects)) {
		each := make([]string, 0, len(effects[tool]))
		for _, effect := range effects[tool] {
			if effect == nil {
				each = append(each, "none")
				continue
			}
			each = append(each, effect.String())
		}
		parts = append(parts, tool+"=["+strings.Join(each, ", ")+"]")
	}
	return "committed " + strings.Join(parts, " ")
}
