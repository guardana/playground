package main

import (
	"errors"
	"testing"
)

// The persona is the scenario's to leave out; where the journal goes is not.
func TestStartsOnlyWhenItKnowsWhereToJournal(t *testing.T) {
	full := map[string]string{"LAB_RUN_ID": "r", "LAB_REPORTS_DIR": "/reports"}
	if _, err := parseSettings([]string{"-personas", "/personas"}, lookupFrom(full)); err != nil {
		t.Fatalf("refused a run with no persona: %v", err)
	}
	for name, args := range map[string][]string{
		"no -personas":    nil,
		"an argument":     {"-personas", "/personas", "extra"},
		"an unknown flag": {"-personas", "/personas", "-persona", "honest"},
	} {
		if _, err := parseSettings(args, lookupFrom(full)); !errors.Is(err, errInvalidConfig) {
			t.Errorf("%s: got %v, want errInvalidConfig", name, err)
		}
	}
	for _, missing := range []string{"LAB_RUN_ID", "LAB_REPORTS_DIR"} {
		env := map[string]string{"LAB_RUN_ID": "r", "LAB_REPORTS_DIR": "/reports"}
		env[missing] = " "
		if _, err := parseSettings([]string{"-personas", "/personas"}, lookupFrom(env)); !errors.Is(err, errInvalidConfig) {
			t.Errorf("%s blank: got %v, want errInvalidConfig", missing, err)
		}
	}
}

func TestAPersonaNameCannotLeaveTheDirectory(t *testing.T) {
	for name, plain := range map[string]bool{
		"honest": true, "no-pkce": true, "v2": true,
		"": false, "-x": false, "x-": false, "../honest": false, "a/b": false,
		"Honest": false, "honest.json": false, "a b": false,
	} {
		if got := plainName(name); got != plain {
			t.Errorf("plainName(%q) = %v, want %v", name, got, plain)
		}
	}
}
