package main

import (
	"maps"
	"slices"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

// Every victim a trajectory may call has a sink, and nothing else has one, so
// a victim added to the lab cannot reach a trace as "other" unnoticed.
func TestEveryVictimHasASink(t *testing.T) {
	got := slices.Sorted(maps.Keys(sinks()))
	want := slices.Sorted(slices.Values(labspec.Victims()))
	if !slices.Equal(got, want) {
		t.Errorf("sinks name %v, the lab's victims are %v", got, want)
	}
}

func TestAServerWithNoSinkIsAnError(t *testing.T) {
	if sink, err := sinkOf("victim-unlisted"); err == nil {
		t.Errorf("victim-unlisted has sink %q, want an error", sink)
	}
	w := &traceWriter{namespace: "lab"}
	if err := w.record(1, traceStep("victim-unlisted", "x.y"), map[string]any{}, nil, false); err == nil {
		t.Error("a step to a server with no sink was recorded")
	}
}
