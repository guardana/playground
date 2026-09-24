package check_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

func line(runID, tool string, status journal.Status, detail string) journal.Entry {
	return journal.Entry{
		OccurredAt: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
		Server:     "victim-fs",
		Tool:       tool,
		RunID:      runID,
		Status:     status,
		Detail:     detail,
	}
}

func gradeFS(t *testing.T, expect labspec.EffectExpectation, entries ...journal.Entry) assertion.Result {
	t.Helper()
	spec := scenario(map[int]labspec.DecisionExpectation{1: {Verdict: "ALLOW"}})
	spec.Expect.Effects = map[string]labspec.EffectExpectation{"victim-fs": expect}
	results, err := check.Effects{Scenario: spec, JournalDir: journalDir}.Run(context.Background(),
		records(nil, map[string][]journal.Entry{"victim-fs": entries}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want one", len(results))
	}
	return results[0]
}

// A call the victim received and refused is a call that got past the gateway.
// A scenario that names only what was served would pass a run in which the
// gateway let through a call the victim happened to turn away.
func TestEffectsAccountForEveryLineOfTheRun(t *testing.T) {
	outside := line(thisRun, "fs.read", journal.Refused, "path /etc/passwd is outside the sandbox")
	tests := []struct {
		name    string
		expect  labspec.EffectExpectation
		entries []journal.Entry
		want    assertion.Outcome
		quotes  []string
	}{
		{
			name:    "a refused line the scenario does not name fails and is quoted",
			expect:  labspec.EffectExpectation{CallsServed: map[string]int{}},
			entries: []journal.Entry{outside},
			want:    assertion.Fail,
			quotes:  []string{"fs.read refused 1", `"status":"refused"`, "/etc/passwd is outside the sandbox"},
		},
		{
			name:    "a refused line counted under calls_served fails",
			expect:  labspec.EffectExpectation{CallsServed: map[string]int{"fs.read": 1}},
			entries: []journal.Entry{outside},
			want:    assertion.Fail,
			quotes:  []string{"fs.read served 0", "fs.read refused 1", "/etc/passwd"},
		},
		{
			name:    "a refused line the scenario names passes",
			expect:  labspec.EffectExpectation{CallsServed: map[string]int{}, CallsRefused: map[string]int{"fs.read": 1}},
			entries: []journal.Entry{outside},
			want:    assertion.Pass,
		},
		{
			name:    "a refusal the scenario names and the victim never made fails",
			expect:  labspec.EffectExpectation{CallsServed: map[string]int{}, CallsRefused: map[string]int{"fs.read": 1}},
			entries: nil,
			want:    assertion.Fail,
			quotes:  []string{"fs.read refused 0 call(s), the scenario expects 1"},
		},
		{
			name:    "one refusal too many of a tool the scenario names fails",
			expect:  labspec.EffectExpectation{CallsServed: map[string]int{}, CallsRefused: map[string]int{"fs.read": 1}},
			entries: []journal.Entry{outside, outside},
			want:    assertion.Fail,
			quotes:  []string{"fs.read refused 2 call(s), the scenario expects 1"},
		},
		{
			name:    "a served line the scenario does not name is quoted",
			expect:  labspec.EffectExpectation{CallsServed: map[string]int{}},
			entries: []journal.Entry{line(thisRun, "fs.write", journal.Served, "wrote /data/a.txt")},
			want:    assertion.Fail,
			quotes:  []string{"fs.write served 1", `"status":"served"`, "/data/a.txt"},
		},
		{
			name: "a status the lab does not know fails and is quoted",
			expect: labspec.EffectExpectation{
				CallsServed: map[string]int{"fs.read": 1}, CallsRefused: map[string]int{"fs.read": 1},
			},
			entries: []journal.Entry{
				line(thisRun, "fs.read", journal.Served, ""),
				line(thisRun, "fs.read", journal.Refused, ""),
				line(thisRun, "fs.read", "pending", "queued"),
			},
			want:   assertion.Fail,
			quotes: []string{`status "pending"`, `"status":"pending"`},
		},
		{
			name:    "a refused line of another run is not counted",
			expect:  labspec.EffectExpectation{CallsServed: map[string]int{}},
			entries: []journal.Entry{line("run-yesterday", "fs.read", journal.Refused, "")},
			want:    assertion.Pass,
			quotes:  []string{"1 line(s) recorded in another run were not counted"},
		},
		{
			name:    "a line of another run with an unknown status is not read",
			expect:  labspec.EffectExpectation{CallsServed: map[string]int{}},
			entries: []journal.Entry{line("run-yesterday", "fs.read", "pending", "")},
			want:    assertion.Pass,
		},
		{
			name:    "a named refusal does not stand in for another run's",
			expect:  labspec.EffectExpectation{CallsServed: map[string]int{}, CallsRefused: map[string]int{"fs.read": 1}},
			entries: []journal.Entry{line("run-yesterday", "fs.read", journal.Refused, "")},
			want:    assertion.Fail,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := gradeFS(t, test.expect, test.entries...)
			if result.Outcome != test.want {
				t.Errorf("outcome %s, want %s (%+v)", result.Outcome, test.want, result)
			}
			for _, quote := range test.quotes {
				if !strings.Contains(result.Detail, quote) {
					t.Errorf("detail %q does not say %q", result.Detail, quote)
				}
			}
		})
	}
}

func TestEffectsWantAndGotNameBothStatuses(t *testing.T) {
	result := gradeFS(t,
		labspec.EffectExpectation{CallsServed: map[string]int{"fs.read": 1}, CallsRefused: map[string]int{"fs.list": 1}},
		line(thisRun, "fs.read", journal.Served, ""),
		line(thisRun, "fs.list", journal.Refused, ""),
	)
	if want := "served fs.read=1; refused fs.list=1"; result.Want != want {
		t.Errorf("want %q, want %q", result.Want, want)
	}
	if result.Got != result.Want {
		t.Errorf("got %q, want %q", result.Got, result.Want)
	}
	nothing := gradeFS(t, labspec.EffectExpectation{CallsServed: map[string]int{}})
	if want := "nothing served; nothing refused"; nothing.Want != want || nothing.Got != want {
		t.Errorf("want %q got %q, both should be %q", nothing.Want, nothing.Got, want)
	}
}

// A journal line may be a megabyte long; the report quotes enough of it to
// find the line and no more.
func TestAQuotedLineCarriesABoundedDetail(t *testing.T) {
	long := strings.Repeat("x", 4096)
	got := gradeFS(t, labspec.EffectExpectation{CallsServed: map[string]int{}},
		line(thisRun, "fs.read", journal.Refused, long))
	if got.Outcome != assertion.Fail {
		t.Fatalf("an unnamed refusal was %s", got.Outcome)
	}
	if strings.Contains(got.Detail, long) || !strings.Contains(got.Detail, strings.Repeat("x", 512)+"…") {
		t.Errorf("the quoted detail is not cut at 512 bytes: %d bytes of detail", len(got.Detail))
	}
}
