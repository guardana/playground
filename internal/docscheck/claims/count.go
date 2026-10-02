package claims

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	countHome        = "docs/status.md"
	scenarioPattern  = "scenarios/*/*.yaml"
	changelog        = "CHANGELOG.md"
	countPlaceReason = "states a scenario count (%s); it lives in " + countHome + " only"
)

// scenarioCount matches a number, up to two words, then "scenario" or
// "scenarios". The number may not end an identifier such as chaos-02; no word
// ends a sentence or crosses a table cell; the gaps stay on one line or span
// one line break.
var scenarioCount = regexp.MustCompile(`(?im)(?:^|[^\w./#-])(\d+)` +
	`(?:(?:[ \t]*\n[ \t]*|[ \t]+)[^\s|]*[^\s|.!?]){0,2}` +
	`(?:[ \t]*\n[ \t]*|[ \t]+)scenarios?\b`)

// ScenarioCount holds the scenario count to one home: in every listed
// markdown file but the changelog, which is history, a scenario count may
// appear only in docs/status.md, which must state it at least once, and there
// every count must be the number of tracked files matching scenarios/*/*.yaml.
// A tracked list holding no scenario, or a listed one without docs/status.md,
// is refused: the check would then have judged nothing.
func ScenarioCount(fsys fs.FS, listed, tracked []string) ([]Problem, error) {
	want := scenarioTotal(tracked)
	if want == 0 {
		return nil, fmt.Errorf("claims: no tracked file matches %s", scenarioPattern)
	}
	if !slices.Contains(listed, countHome) {
		return nil, fmt.Errorf("claims: %s is not among the files to check", countHome)
	}
	var problems []Problem
	stated := false
	for _, rel := range listed {
		if !strings.HasSuffix(rel, ".md") || rel == changelog {
			continue
		}
		data, err := fs.ReadFile(fsys, rel)
		if err != nil {
			return nil, err
		}
		found, ok := judgeCounts(rel, data, want)
		problems = append(problems, found...)
		stated = stated || ok
	}
	if !stated {
		problems = append(problems, Problem{Path: countHome, Line: 1,
			Reason: fmt.Sprintf("never states the total (%d) of %s", want, scenarioPattern)})
	}
	return problems, nil
}

func scenarioTotal(tracked []string) int {
	total := 0
	for _, rel := range tracked {
		if ok, _ := path.Match(scenarioPattern, rel); ok {
			total++
		}
	}
	return total
}

// judgeCounts reports every count in one page, and whether the page states
// the total where it belongs.
func judgeCounts(rel string, data []byte, want int) ([]Problem, bool) {
	var problems []Problem
	stated := false
	for _, c := range counts(data) {
		switch {
		case rel != countHome:
			problems = append(problems, Problem{Path: rel, Line: c.line, Reason: fmt.Sprintf(countPlaceReason, c.digits)})
		case c.value != want:
			problems = append(problems, Problem{Path: rel, Line: c.line,
				Reason: fmt.Sprintf("says %s scenarios; %s holds %d", c.digits, scenarioPattern, want)})
		default:
			stated = true
		}
	}
	return problems, stated
}

type count struct {
	line   int
	digits string
	value  int
}

// counts returns every scenario count in data; a number too large to read
// keeps value -1, so it never equals a total.
func counts(data []byte) []count {
	var found []count
	for _, loc := range scenarioCount.FindAllSubmatchIndex(data, -1) {
		digits := string(data[loc[2]:loc[3]])
		value, err := strconv.Atoi(digits)
		if err != nil {
			value = -1
		}
		line := 1 + strings.Count(string(data[:loc[2]]), "\n")
		found = append(found, count{line: line, digits: digits, value: value})
	}
	return found
}
