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

const journalDir = "reports/run-1/journals"

func TestEffectsCountWhatTheVictimRecorded(t *testing.T) {
	tests := []struct {
		name     string
		expect   map[string]labspec.EffectExpectation
		journals map[string][]journal.Entry
		want     assertion.Outcome
		detail   string
	}{
		{
			name:     "the counts the scenario names",
			expect:   map[string]labspec.EffectExpectation{"victim-fs": {CallsServed: map[string]int{"fs.read": 1}}},
			journals: map[string][]journal.Entry{"victim-fs": {served("victim-fs", "fs.read")}},
			want:     assertion.Pass,
		},
		{
			name:     "an empty map is the assertion that the victim served nothing",
			expect:   map[string]labspec.EffectExpectation{"victim-mail": {CallsServed: map[string]int{}}},
			journals: map[string][]journal.Entry{"victim-mail": nil},
			want:     assertion.Pass,
		},
		{
			name:     "a call that arrived where the scenario expected none",
			expect:   map[string]labspec.EffectExpectation{"victim-mail": {CallsServed: map[string]int{}}},
			journals: map[string][]journal.Entry{"victim-mail": {served("victim-mail", "mail.send")}},
			want:     assertion.Fail,
			detail:   "mail.send",
		},
		{
			name:   "a tool served that the scenario did not name",
			expect: map[string]labspec.EffectExpectation{"victim-fs": {CallsServed: map[string]int{"fs.read": 1}}},
			journals: map[string][]journal.Entry{"victim-fs": {
				served("victim-fs", "fs.read"),
				served("victim-fs", "fs.write"),
			}},
			want:   assertion.Fail,
			detail: "fs.write",
		},
		{
			name:   "one call too many of a tool the scenario did name",
			expect: map[string]labspec.EffectExpectation{"victim-mail": {CallsServed: map[string]int{"mail.send": 1}}},
			journals: map[string][]journal.Entry{"victim-mail": {
				served("victim-mail", "mail.send"),
				served("victim-mail", "mail.send"),
			}},
			want:   assertion.Fail,
			detail: "mail.send",
		},
		{
			name:     "a journal that is absent is a failure and names the victim",
			expect:   map[string]labspec.EffectExpectation{"victim-shell": {CallsServed: map[string]int{}}},
			journals: nil,
			want:     assertion.Fail,
			detail:   "victim-shell",
		},
		{
			name: "a refused call is not a served call",
			expect: map[string]labspec.EffectExpectation{"victim-fs": {
				CallsServed: map[string]int{}, CallsRefused: map[string]int{"fs.read": 1},
			}},
			journals: map[string][]journal.Entry{"victim-fs": {{
				OccurredAt: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
				Server:     "victim-fs",
				Tool:       "fs.read",
				Status:     journal.Refused,
			}}},
			want: assertion.Pass,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := scenario(map[int]labspec.DecisionExpectation{1: {Verdict: "ALLOW"}})
			spec.Expect.Effects = test.expect
			checker := check.Effects{Scenario: spec, JournalDir: journalDir}

			results, err := checker.Run(context.Background(), records(nil, test.journals))
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(results) != len(test.expect) {
				t.Fatalf("got %d results, want one per victim the scenario names", len(results))
			}
			if results[0].Outcome != test.want {
				t.Errorf("outcome %s, want %s (%+v)", results[0].Outcome, test.want, results[0])
			}
			if test.detail != "" && !strings.Contains(results[0].Detail, test.detail) {
				t.Errorf("detail %q does not name %q", results[0].Detail, test.detail)
			}
		})
	}
}

func TestEffectsSourceNamesTheJournalItRead(t *testing.T) {
	spec := scenario(map[int]labspec.DecisionExpectation{1: {Verdict: "ALLOW"}})
	spec.Expect.Effects = map[string]labspec.EffectExpectation{
		"victim-fs":  {CallsServed: map[string]int{"fs.read": 1}},
		"victim-web": {CallsServed: map[string]int{"web.fetch": 1}},
	}
	given := map[string][]journal.Entry{
		"victim-fs":  {served("victim-fs", "fs.read")},
		"victim-web": {served("victim-web", "web.fetch")},
	}
	checker := check.Effects{Scenario: spec, JournalDir: journalDir}

	results, err := checker.Run(context.Background(), records(nil, given))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Sorted by victim, so two runs over one set of records report in one order.
	want := []string{journalDir + "/victim-fs.jsonl", journalDir + "/victim-web.jsonl"}
	for i, result := range results {
		if result.Source != want[i] {
			t.Errorf("result %d source is %q, want %q", i, result.Source, want[i])
		}
	}
}

// A journal is appended to, and a line carries the run it was served in. An
// entry from an earlier run counted into this one would let a victim that
// served nothing report a call, and a denial that held report a call it never
// served.
func TestEffectsCountOnlyTheCallsServedInThisRun(t *testing.T) {
	tests := []struct {
		name    string
		expect  map[string]int
		entries []journal.Entry
		want    assertion.Outcome
	}{
		{
			name:    "a call from another run does not stand in for this run's",
			expect:  map[string]int{"fs.read": 1},
			entries: []journal.Entry{servedIn("run-yesterday", "victim-fs", "fs.read")},
			want:    assertion.Fail,
		},
		{
			name:    "a call from another run is not a call this run's denial let through",
			expect:  map[string]int{},
			entries: []journal.Entry{servedIn("run-yesterday", "victim-fs", "fs.read")},
			want:    assertion.Pass,
		},
		{
			name:    "this run's own call is counted",
			expect:  map[string]int{"fs.read": 1},
			entries: []journal.Entry{served("victim-fs", "fs.read")},
			want:    assertion.Pass,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := scenario(map[int]labspec.DecisionExpectation{1: {Verdict: "ALLOW"}})
			spec.Expect.Effects = map[string]labspec.EffectExpectation{"victim-fs": {CallsServed: test.expect}}
			checker := check.Effects{Scenario: spec, JournalDir: journalDir}

			results, err := checker.Run(context.Background(),
				records(nil, map[string][]journal.Entry{"victim-fs": test.entries}))
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if results[0].Outcome != test.want {
				t.Errorf("outcome %s, want %s (%+v)", results[0].Outcome, test.want, results[0])
			}
		})
	}
}
