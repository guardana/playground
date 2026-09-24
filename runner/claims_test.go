package main

import (
	"context"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
)

// A configuration that ran the enforcer in another mode than the scenario
// states is a run of another scenario, whatever its decisions say.
func TestAnEnforcerRunUnderAnotherModeIsRed(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	unstamped := strings.NewReplacer(`"runId":"${RUN_ID}",`, "", `"runId":"${RUN_ID}"`, "").Replace(evidenceFile)
	observed := strings.ReplaceAll(asEnforced(unstamped), "ENFORCEMENT_MODE_ENFORCE", "ENFORCEMENT_MODE_OBSERVE")
	compose.collector = otlpOf(t, observed)

	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	found := results(graded)
	if found["evidence/enforcement-mode"].Outcome != assertion.Fail || graded.Outcome() != assertion.Fail {
		t.Fatalf("mode %s, run %s: %+v", found["evidence/enforcement-mode"].Outcome, graded.Outcome(),
			found["evidence/enforcement-mode"])
	}
	if found["decisions/step-1"].Outcome != assertion.Pass {
		t.Errorf("the decision itself is %s; only the mode should be red", found["decisions/step-1"].Outcome)
	}
}

// What ran is compared with what was decided on every run the enforcer decides.
func TestAnEnforcerRunThatRanOtherBytesIsRed(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	unstamped := strings.NewReplacer(`"runId":"${RUN_ID}",`, "", `"runId":"${RUN_ID}"`, "").Replace(evidenceFile)
	altered := strings.Replace(asEnforced(unstamped), `"executedActionDigest":"sha256:1111`, `"executedActionDigest":"sha256:2222`, 1)
	compose.collector = otlpOf(t, altered)

	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := results(graded)["evidence/executed-digest"]; got.Outcome != assertion.Fail {
		t.Fatalf("executed digest is %s: %+v", got.Outcome, got)
	}
}
