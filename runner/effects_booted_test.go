package main

import (
	"context"
	"testing"

	"github.com/guardana/playground/internal/assertion"
)

// Every victim of the profile boots whatever the trajectory calls, so one the
// scenario does not name is held to having served nothing: a call it served
// that no trail shows is a call the run made and nobody looked at.
func TestATrajectoryRunHoldsEveryBootedVictimToWhatItServed(t *testing.T) {
	for name, tc := range map[string]struct {
		journal string
		want    assertion.Outcome
	}{
		"an unnamed victim that served a call": {
			`{"occurred_at":"2026-09-09T12:00:03Z","server":"victim-shell","tool":"shell.exec","run_id":"${RUN_ID}","status":"served"}` + "\n",
			assertion.Fail,
		},
		"an unnamed victim that served nothing": {"", assertion.Pass},
	} {
		t.Run(name, func(t *testing.T) {
			subject, compose, scenario := enforcerLab(t)
			compose.inProfile = append(compose.inProfile, "victim-shell")
			compose.status = append(compose.status, assertion.Service{Name: "victim-shell", Running: true, Detail: "running"})
			compose.journals["victim-shell"] = tc.journal
			graded, err := subject.execute(context.Background(), scenario)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			got, graded2 := results(graded)["effects/victim-shell"]
			if !graded2 || got.Outcome != tc.want {
				t.Errorf("effects/victim-shell = %v %+v, want %s", graded2, got, tc.want)
			}
		})
	}
}
