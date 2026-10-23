package check_test

import (
	"time"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/internal/labspec"
)

// The builders below assemble a victim's journal by hand rather than by running
// anything, because the checks are what decides whether the lab tells the truth
// and a check tested against its own producer would only prove the two agree.

// thisRun is the lab's run the records name, which the victims stamp their
// journals with.
const thisRun = "run-1"

func records(journals map[string][]journal.Entry) assertion.Records {
	return assertion.Records{
		RunID:    thisRun,
		Scenario: "scenario-1",
		Boot:     assertion.Boot{Profile: "core"},
		Journals: journals,
	}
}

// served is one journal line for a call the victim answered, stamped with the
// run the records name.
func served(server, tool string) journal.Entry {
	return servedIn(thisRun, server, tool)
}

func servedIn(runID, server, tool string) journal.Entry {
	return journal.Entry{
		OccurredAt: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
		Server:     server,
		Tool:       tool,
		RunID:      runID,
		Status:     journal.Served,
	}
}

func scenario(decisions map[int]labspec.DecisionExpectation) labspec.Scenario {
	return labspec.Scenario{
		SchemaVersion:   labspec.SchemaVersion,
		ID:              "scenario-1",
		Title:           "a scenario built for a check test",
		Profile:         []string{"core"},
		EnforcementMode: "enforce",
		Trajectory:      "trajectories/scenario-1.yaml",
		Expect:          labspec.Expect{Decisions: decisions, Evidence: &labspec.EvidenceExpectation{}},
	}
}
