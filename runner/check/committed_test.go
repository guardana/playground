package check_test

import (
	"context"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

func charged(amount int64) journal.Effect {
	return journal.Effect{"amount": journal.Integer(amount), "currency": journal.String("EUR")}
}

func committedIn(runID string, status journal.Status, effect journal.Effect) journal.Entry {
	entry := servedIn(runID, "victim-pay", "pay.charge")
	entry.Status, entry.Effect = status, effect
	return entry
}

func committedResult(t *testing.T, want map[string][]journal.Effect, served int, entries []journal.Entry, collected bool) (assertion.Result, bool) {
	t.Helper()
	expect := labspec.EffectExpectation{CallsServed: map[string]int{"pay.charge": served}, Committed: want}
	checker := check.Effects{
		Scenario:   labspec.Scenario{Expect: labspec.Expect{Effects: map[string]labspec.EffectExpectation{"victim-pay": expect}}},
		JournalDir: journalDir,
	}
	journals := map[string][]journal.Entry{}
	if collected {
		journals["victim-pay"] = entries
	}
	results, err := checker.Run(context.Background(), records(nil, journals))
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.Check == "effects/victim-pay/committed" {
			return result, true
		}
	}
	return assertion.Result{}, false
}

func TestCommittedGradesEachEffectInOrder(t *testing.T) {
	stated := map[string][]journal.Effect{"pay.charge": {charged(5000), charged(3000)}}
	served := func(effects ...journal.Effect) []journal.Entry {
		var entries []journal.Entry
		for _, effect := range effects {
			entries = append(entries, committedIn(thisRun, journal.Served, effect))
		}
		return entries
	}
	tests := []struct {
		name    string
		want    map[string][]journal.Effect
		served  int
		entries []journal.Entry
		outcome assertion.Outcome
		detail  string
	}{
		{"the amounts the scenario states", stated, 2, served(charged(5000), charged(3000)), assertion.Pass, ""},
		{"an amount the cap should have lowered", stated, 2, served(charged(50000), charged(3000)), assertion.Fail,
			`pay.charge #1 committed {"amount":50000,"currency":"EUR"}, the scenario expects {"amount":5000,"currency":"EUR"}`},
		{"the right amounts in the other order", stated, 2, served(charged(3000), charged(5000)), assertion.Fail, "#1 committed"},
		{"a served call with no effect recorded", stated, 2, served(charged(5000), nil), assertion.Fail,
			"pay.charge #2 was served with no effect recorded"},
		{"one call more than stated", stated, 2, served(charged(5000), charged(3000), charged(1)), assertion.Fail,
			"pay.charge served 3 call(s), the scenario states 2 effect(s)"},
		{"the same number written as a string", map[string][]journal.Effect{"pay.charge": {{"amount": journal.String("5000")}}}, 1,
			served(journal.Effect{"amount": journal.Integer(5000)}), assertion.Fail, "#1 committed"},
		{"a member more than stated", map[string][]journal.Effect{"pay.charge": {{"amount": journal.Integer(5000)}}}, 1,
			served(charged(5000)), assertion.Fail, "#1 committed"},
		{"an effect where the scenario states none", nil, 1, served(charged(5000)), assertion.Fail,
			"pay.charge #1 committed {\"amount\":5000,\"currency\":\"EUR\"}, which the scenario does not name"},
		{"an effect on a refused line", nil, 0, []journal.Entry{committedIn(thisRun, journal.Refused, charged(5000))},
			assertion.Fail, `a "refused" line carries an effect`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, graded := committedResult(t, test.want, test.served, test.entries, true)
			if !graded {
				t.Fatal("no effects/victim-pay/committed result")
			}
			if result.Outcome != test.outcome {
				t.Errorf("outcome = %v, want %v (got %q, detail %q)", result.Outcome, test.outcome, result.Got, result.Detail)
			}
			if !strings.Contains(result.Detail, test.detail) {
				t.Errorf("detail = %q, want it to name %q", result.Detail, test.detail)
			}
		})
	}
}

func TestCommittedSaysWhatItCompared(t *testing.T) {
	result, _ := committedResult(t, map[string][]journal.Effect{"pay.charge": {charged(5000)}}, 1,
		[]journal.Entry{committedIn(thisRun, journal.Served, nil)}, true)
	if want := `committed pay.charge=[{"amount":5000,"currency":"EUR"}]`; result.Want != want {
		t.Errorf("want = %q, want %q", result.Want, want)
	}
	if got := "committed pay.charge=[none]"; result.Got != got {
		t.Errorf("got = %q, want %q", result.Got, got)
	}
	if result.Source != journalDir+"/victim-pay.jsonl" {
		t.Errorf("source = %q", result.Source)
	}
}

// A statement about a journal nobody read fails; with nothing stated and
// nothing recorded there is nothing to grade, and no result says otherwise.
func TestCommittedWithoutAJournalOrAnEffect(t *testing.T) {
	result, graded := committedResult(t, map[string][]journal.Effect{"pay.charge": {charged(5000)}}, 1, nil, false)
	if !graded || result.Outcome != assertion.Fail || result.Got != "no journal" {
		t.Errorf("stated, no journal: graded %v, %+v", graded, result)
	}
	if _, graded := committedResult(t, nil, 0, nil, false); graded {
		t.Error("nothing stated and no journal produced a committed result")
	}
	entries := []journal.Entry{servedIn(thisRun, "victim-pay", "pay.read_charge")}
	if _, graded := committedResult(t, nil, 0, entries, true); graded {
		t.Error("nothing stated and no effect produced a committed result")
	}
}

// A journal outlives a run: another run's charge is neither graded nor hidden.
func TestCommittedReadsOnlyThisRun(t *testing.T) {
	entries := []journal.Entry{
		committedIn("run-0", journal.Served, charged(50000)),
		committedIn(thisRun, journal.Served, charged(5000)),
	}
	result, _ := committedResult(t, map[string][]journal.Effect{"pay.charge": {charged(5000)}}, 1, entries, true)
	if result.Outcome != assertion.Pass {
		t.Fatalf("outcome = %v, detail %q", result.Outcome, result.Detail)
	}
	if !strings.Contains(result.Detail, "1 line(s) recorded in another run were not graded") {
		t.Errorf("detail = %q, want the other run's line counted", result.Detail)
	}
}
