package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheRunnerRefusesAnEnvironmentThatOverridesAPin(t *testing.T) {
	root := t.TempDir()
	writeFile(filepath.Join(root, versionFile), "# pins\nENFORCER_COMMIT=c0ffee\nVERIFIER_VERSION=0.26.1\n")
	for _, c := range []struct {
		name    string
		environ []string
		refused string
	}{
		{name: "another commit", environ: []string{"HOME=/h", "ENFORCER_COMMIT=deadbeef"}, refused: "ENFORCER_COMMIT"},
		{name: "set but empty", environ: []string{"VERIFIER_VERSION="}, refused: "VERIFIER_VERSION"},
		{name: "the same value", environ: []string{"ENFORCER_COMMIT=c0ffee"}, refused: "ENFORCER_COMMIT"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := refuseOverriddenPins(root, c.environ)
			if err == nil || !strings.Contains(err.Error(), c.refused) {
				t.Errorf("refuseOverriddenPins = %v, want a refusal naming %s", err, c.refused)
			}
		})
	}
}

func TestTheRunnerStartsWhenNoPinIsOverridden(t *testing.T) {
	root := t.TempDir()
	writeFile(filepath.Join(root, versionFile), "ENFORCER_COMMIT=c0ffee\n")
	environ := []string{"HOME=/h", "ENFORCER_SOURCE=/src", "ENFORCER_COMMIT_NOTE=x", "PATH=/bin"}
	if err := refuseOverriddenPins(root, environ); err != nil {
		t.Errorf("an environment that sets no pin was refused: %v", err)
	}
}

func TestTheRunnerRefusesWithoutVersionsFile(t *testing.T) {
	if err := refuseOverriddenPins(t.TempDir(), nil); err == nil {
		t.Error("a checkout without versions.env was accepted")
	}
}

func TestRunRefusesBeforeLookingForAScenario(t *testing.T) {
	root := t.TempDir()
	writeFile(filepath.Join(root, versionFile), "ENFORCER_COMMIT=c0ffee\n")
	t.Chdir(root)
	var out strings.Builder
	err := run(context.Background(), []string{"-scenario", "flow-01"}, []string{"ENFORCER_COMMIT=deadbeef"}, &out)
	if err == nil || !strings.Contains(err.Error(), "ENFORCER_COMMIT") {
		t.Errorf("run = %v, want a refusal naming ENFORCER_COMMIT", err)
	}
}
