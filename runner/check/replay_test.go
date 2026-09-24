package check_test

import (
	"context"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/runner/check"
)

func TestReplayGradesWhetherTheTrajectoryWasSent(t *testing.T) {
	tests := []struct {
		name    string
		checker check.Replay
		want    assertion.Outcome
	}{
		{
			name:    "the agent sent every step",
			checker: check.Replay{Ran: true, Source: "reports/run-1/replay.log"},
			want:    assertion.Pass,
		},
		{
			name:    "a step could not be sent",
			checker: check.Replay{Ran: true, ExitCode: 1, Detail: "step 3: connection reset"},
			want:    assertion.Fail,
		},
		{
			name:    "the agent could not be run at all",
			checker: check.Replay{Detail: "no such image"},
			want:    assertion.Indeterminate,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			results, err := test.checker.Run(context.Background(), records(nil, nil))
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("got %d results, want one", len(results))
			}
			if results[0].Outcome != test.want {
				t.Errorf("outcome %s, want %s (%+v)", results[0].Outcome, test.want, results[0])
			}
		})
	}
}
