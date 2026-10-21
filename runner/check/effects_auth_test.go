package check_test

import (
	"context"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

func authScenario(want ...labspec.AuthExpectation) labspec.Scenario {
	return labspec.Scenario{Expect: labspec.Expect{Effects: map[string]labspec.EffectExpectation{
		"victim-vault": {CallsServed: map[string]int{}, AuthInclude: want},
	}}}
}

func authDecision(run string, credential journal.Credential, session journal.Session, status int) journal.Auth {
	return journal.Auth{Server: "victim-vault", RunID: run, Credential: credential, Session: session, Status: status}
}

func gradeAuth(t *testing.T, scenario labspec.Scenario, records assertion.Records) assertion.Result {
	t.Helper()
	results, err := check.Effects{Scenario: scenario, JournalDir: t.TempDir()}.Run(context.Background(), records)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.Check == "effects/victim-vault/auth" {
			return result
		}
	}
	t.Fatalf("no effects/victim-vault/auth among %d results", len(results))
	return assertion.Result{}
}

func TestEachAuthDecisionAScenarioWantsIsReadFromThisRunsLines(t *testing.T) {
	lines := []journal.Auth{
		authDecision("run-1", journal.CredentialRun, journal.SessionNone, 200),
		authDecision("run-1", journal.CredentialOther, journal.SessionNone, 401),
		authDecision("run-0", journal.CredentialNone, journal.SessionKnown, 200),
	}
	records := assertion.Records{RunID: "run-1",
		Journals: map[string][]journal.Entry{"victim-vault": nil},
		Auth:     map[string][]journal.Auth{"victim-vault": lines}}
	run := labspec.AuthExpectation{Credential: "run"}
	other := labspec.AuthExpectation{Credential: "other", Status: 401}
	for name, test := range map[string]struct {
		want    []labspec.AuthExpectation
		outcome assertion.Outcome
	}{
		"every decision recorded":           {[]labspec.AuthExpectation{run, other}, assertion.Pass},
		"a status never answered":           {[]labspec.AuthExpectation{run, {Credential: "other", Status: 200}}, assertion.Fail},
		"a decision only another run made":  {[]labspec.AuthExpectation{{Credential: "none", Session: "known"}}, assertion.Fail},
		"a session the request never named": {[]labspec.AuthExpectation{{Credential: "run", Session: "known"}}, assertion.Fail},
	} {
		t.Run(name, func(t *testing.T) {
			if got := gradeAuth(t, authScenario(test.want...), records); got.Outcome != test.outcome {
				t.Errorf("effects/victim-vault/auth = %s (%q), want %s", got.Outcome, got.Got, test.outcome)
			}
		})
	}
}

// A victim whose authentication was never read has not shown the token
// reached it.
func TestAnAuthExpectationWithNoDecisionsReadFails(t *testing.T) {
	records := assertion.Records{RunID: "run-1", Journals: map[string][]journal.Entry{"victim-vault": nil}}
	got := gradeAuth(t, authScenario(labspec.AuthExpectation{Credential: "run"}), records)
	if got.Outcome != assertion.Fail {
		t.Errorf("effects/victim-vault/auth = %s (%q), want fail", got.Outcome, got.Got)
	}
}
