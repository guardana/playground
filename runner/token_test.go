package main

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/runner/dockercompose"
)

const handedOverScenarioFile = `schema_version: 1
id: verify-token
title: one step is handed the run's token and one is not
profile: [verifier]
auth_double: honest
verifier:
  - probe: { server: victim-fs, credential: true }
  - probe: { server: victim-fs }
expect:
  verifier:
    1: { exit_code: 1 }
    2: { exit_code: 1 }
  effects:
    victim-fs: { calls_served: {} }
`

func credentialUnderTest(t *testing.T) (lab, *fakeCompose, string) {
	t.Helper()
	subject, compose, _ := verifierUnderTest(t)
	scenario := filepath.Join(subject.root, "scenarios/verify/verify-token.yaml")
	writeFile(scenario, handedOverScenarioFile)
	writeFile(filepath.Join(subject.root, "config/auth/honest.json"), honestPersona)
	return subject, compose, scenario
}

// honestPersona stands for a persona file; the double, not the runner, reads it.
const honestPersona = `{"issuer":"http://auth-double:8080"}`

func TestOnlyACredentialStepIsHandedTheRunsTokenAndOnlyByName(t *testing.T) {
	subject, compose, scenario := credentialUnderTest(t)
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	wantPersonaHandedOver(t, filepath.Join(subject.reports, graded.RunID, personaDir, "honest.json"))
	token := compose.env[mcpTokenVariable]
	if raw, err := hex.DecodeString(token); err != nil || len(raw) != 32 {
		t.Fatalf("%s is %d characters of hex %t, want 32 random bytes", mcpTokenVariable, len(token), err == nil)
	}
	if persona := compose.env[authPersonaVariable]; persona != "honest" {
		t.Errorf("%s = %q, want the scenario's persona", authPersonaVariable, persona)
	}
	wantOnlyTheFirstStepHandedTheToken(t, compose, token)
	again, compose2, scenario2 := credentialUnderTest(t)
	if _, err := again.execute(context.Background(), scenario2); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if compose2.env[mcpTokenVariable] == token {
		t.Error("two runs were handed the same token")
	}
}

func TestACredentialStepWithNoTokenToHandOverDoesNotRun(t *testing.T) {
	subject, compose, scenario := credentialUnderTest(t)
	subject.token = func() (string, error) { return "", nil }
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	ran, recorded := results(graded)["verifier/step-1/ran"]
	if !recorded || ran.Outcome == assertion.Pass || !strings.Contains(ran.Detail, "token") {
		t.Errorf("step-1/ran = %s %q, want a step that did not run for want of a token", ran.Outcome, ran.Detail)
	}
	for _, call := range compose.splits {
		if slices.Contains(call.args, "--mcp-token-env") {
			t.Errorf("the credential step ran: %v", call.args)
		}
	}
}

// A file of the run that holds the token fails the run, whoever wrote it.
func TestATokenLeftInTheRunsFilesFailsTheRun(t *testing.T) {
	subject, compose, scenario := credentialUnderTest(t)
	working := compose.split
	compose.split = func(service, entrypoint string, args []string) (dockercompose.Split, error) {
		split, err := working(service, entrypoint, args)
		if slices.Contains(args, "--mcp-token-env") {
			split.Stderr += "debug: bearer " + compose.env[mcpTokenVariable] + "\n"
		}
		return split, err
	}
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	kept := results(graded)["verifier/token-kept-out"]
	if kept.Outcome != assertion.Fail || !strings.Contains(kept.Detail, "step-1.stderr") {
		t.Errorf("verifier/token-kept-out = %s %q, want a fail naming step-1.stderr", kept.Outcome, kept.Detail)
	}
	if strings.Contains(kept.Detail+kept.Got+kept.Want, compose.env[mcpTokenVariable]) {
		t.Error("the check repeats the token it found")
	}
	clean, _, scenario2 := credentialUnderTest(t)
	graded, err = clean.execute(context.Background(), scenario2)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if kept := results(graded)["verifier/token-kept-out"]; kept.Outcome != assertion.Pass {
		t.Errorf("a clean run's verifier/token-kept-out = %s %q", kept.Outcome, kept.Detail)
	}
}

// wantOnlyTheFirstStepHandedTheToken holds the first probe step to the
// token's name and the second to none, and every command line to no value.
func wantOnlyTheFirstStepHandedTheToken(t *testing.T, compose *fakeCompose, token string) {
	t.Helper()
	var steps []splitCall
	for _, call := range compose.splits {
		if slices.Contains(call.args, "probe") {
			steps = append(steps, call)
		}
		if strings.Contains(strings.Join(call.args, " "), token) {
			t.Errorf("the token is on a command line: %v", call.args)
		}
	}
	if len(steps) != 2 {
		t.Fatalf("%d probe steps ran, want 2", len(steps))
	}
	first, second := steps[0], steps[1]
	if !slices.Equal(first.runEnv, []string{mcpTokenVariable}) || !hasFlag(first.args, "--mcp-token-env", mcpTokenVariable) {
		t.Errorf("the credential step ran with -e %v and %v", first.runEnv, first.args)
	}
	if len(second.runEnv) != 0 || slices.Contains(second.args, "--mcp-token-env") {
		t.Errorf("a step without credential ran with -e %v and %v", second.runEnv, second.args)
	}
}

// wantPersonaHandedOver holds the run's copy of the persona to the
// workspace's bytes, readable by the double's uid.
func wantPersonaHandedOver(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("the persona was not handed over: %v", err)
	}
	if body, _ := os.ReadFile(path); string(body) != honestPersona || info.Mode() != 0o644 {
		t.Errorf("the run's persona is %v %q, want -rw-r--r-- %q", info.Mode(), body, honestPersona)
	}
}

func hasFlag(args []string, flag, value string) bool {
	i := slices.Index(args, flag)
	return i >= 0 && i+1 < len(args) && args[i+1] == value
}
