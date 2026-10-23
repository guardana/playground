package main

import (
	"time"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/runner/driver"
)

// composeRefused is the report of a run whose compose file did not load for its
// profile, so nothing was built or brought up.
func composeRefused(id, runID string, cause error, at time.Time) assertion.Report {
	return assertion.Report{
		Scenario: id, RunID: runID, StartedAt: at, EndedAt: at,
		Results: []assertion.Result{{
			Check: "compose/loaded", Outcome: assertion.Fail,
			Want: "docker compose reads the topology for the scenario's profile",
			Got:  "the run was refused before anything was built", Detail: cause.Error(),
		}},
	}
}

// unprepared is the report of a run whose environment could not be prepared:
// no key, a policy that does not sign, a configuration the lab refuses, a
// persona or a token that could not be handed over. Nothing was brought up,
// and the run fails rather than skipping.
func unprepared(id, runID string, cause error, at time.Time) assertion.Report {
	return assertion.Report{
		Scenario: id, RunID: runID, StartedAt: at, EndedAt: at,
		Results: []assertion.Result{{
			Check: "plane/prepared", Outcome: assertion.Fail,
			Want: "a signed bundle and an assembled configuration for the enforcer",
			Got:  "the run was refused before anything was brought up", Detail: cause.Error(),
		}},
	}
}

// subjectRefused is the report of a run a subject refused before anything was
// brought up.
func subjectRefused(id, runID string, refusal driver.Refusal, at time.Time) assertion.Report {
	return assertion.Report{
		Scenario: id, RunID: runID, StartedAt: at, EndedAt: at,
		Results: []assertion.Result{{
			Check: refusal.Check, Outcome: assertion.Fail, Want: refusal.Want,
			Got: "the run was refused before anything was brought up", Detail: refusal.Cause.Error(),
		}},
	}
}
