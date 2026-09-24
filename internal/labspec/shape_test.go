package labspec_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/labspec"
)

// heldScenario grades an approval held at step 1, resumed by step 3 after a
// pending retry at step 2 that opens no trail.
const heldScenario = `schema_version: 1
id: stub-01-allow-read
title: a held call resumes once
profile: [core, enforcer]
enforcement_mode: enforce
trajectory: trajectories/stub-01-allow-read.yaml
gateway:
  config: config/gateway/scenarios/stub-01-allow-read.yaml
  policy: config/policies/stub-01-allow-read.json
expect:
  decisions:
    1: { verdict: REQUIRE_APPROVAL, trail: [ACTION_PROPOSED, POLICY_DECIDED, APPROVAL_REQUESTED, APPROVAL_DECIDED, ACTION_STARTED, ACTION_COMPLETED] }
    2: { opens: none }
    3: { resumes: 1, trail: [ACTION_PROPOSED, POLICY_DECIDED, APPROVAL_REQUESTED, APPROVAL_DECIDED, ACTION_STARTED, ACTION_COMPLETED] }
  effects:
    victim-web: { calls_served: { web.fetch: 1 } }
    victim-fs: { calls_served: {} }
    victim-mail: { calls_served: {} }
  evidence: { chain_complete: true, policy_digest_present: true, content_captured: false }
`

func TestAHeldTrailScenarioLoadsAndValidates(t *testing.T) {
	scenario, trajectory := loadPair(t, heldScenario)
	if err := labspec.Validate(scenario, trajectory); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	decisions := scenario.Expect.Decisions
	if !decisions[1].OpensTrail() || decisions[2].OpensTrail() || decisions[3].OpensTrail() {
		t.Errorf("opens = %t %t %t, want true false false",
			decisions[1].OpensTrail(), decisions[2].OpensTrail(), decisions[3].OpensTrail())
	}
	if decisions[3].Resumes != 1 || decisions[2].Opens != labspec.OpensNone {
		t.Errorf("step 2 = %+v, step 3 = %+v", decisions[2], decisions[3])
	}
}

func TestAStepShapeThatGradesNothingIsRefused(t *testing.T) {
	for name, step := range map[string]string{
		"opens none with a verdict":      "2: { opens: none, verdict: ALLOW }",
		"opens none with a trail":        "2: { opens: none, trail: [ACTION_PROPOSED] }",
		"opens something":                "2: { opens: all }",
		"the key writes is gone":         "2: { writes: nothing }",
		"resumes and opens none":         "2: { resumes: 1, opens: none }",
		"resumes stating nothing":        "2: { resumes: 1 }",
		"resumes stating a verdict":      "2: { resumes: 1, verdict: REQUIRE_APPROVAL }",
		"resumes with a verdict beside":  "2: { resumes: 1, verdict: ALLOW, trail: [ACTION_PROPOSED] }",
		"resumes stating reason codes":   "2: { resumes: 1, reason_codes_include: [X], trail: [ACTION_PROPOSED] }",
		"opens without a verdict":        "2: { reason_codes_include: [RULE_ALLOW] }",
		"trail of an unknown kind":       "2: { verdict: ALLOW, trail: [ACTION_PROPOSED, ACTION_TELEPORTED] }",
		"trail not opened by a proposal": "2: { verdict: ALLOW, trail: [POLICY_DECIDED] }",
		"blocked outside its own trail":  "2: { verdict: DENY, blocked: { verdict: DENY }, trail: [ACTION_PROPOSED, POLICY_DECIDED] }",
		"blocked with no verdict":        "2: { verdict: DENY, blocked: { reason_codes_include: [X] } }",
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.Replace(heldScenario, "2: { opens: none }", step, 1)
			_, err := labspec.LoadScenario(writeFile(t, "stub-01-allow-read.yaml", body))
			if !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestAResumeOfATrailNobodyOpenedIsRefused(t *testing.T) {
	for name, replace := range map[string][]string{
		"itself":                 {"3: { resumes: 1,", "3: { resumes: 3,"},
		"a step that opens none": {"3: { resumes: 1,", "3: { resumes: 2,"},
		"a step that runs after it": {
			"2: { opens: none }", "2: { resumes: 3, trail: [ACTION_PROPOSED] }",
			"3: { resumes: 1, trail: [ACTION_PROPOSED, POLICY_DECIDED, APPROVAL_REQUESTED, APPROVAL_DECIDED, ACTION_STARTED, ACTION_COMPLETED] }",
			"3: { verdict: ALLOW }",
		},
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.NewReplacer(replace...).Replace(heldScenario)
			if body == heldScenario {
				t.Fatal("the replacement changed nothing")
			}
			scenario, trajectory := loadPair(t, body)
			if err := labspec.Validate(scenario, trajectory); !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("Validate = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestToleranceOfAStepWithNoVerdictIsRefused(t *testing.T) {
	body := heldScenario + "tolerance: { allow_indeterminate_for_steps: [2] }\n"
	scenario, trajectory := loadPair(t, body)
	if err := labspec.Validate(scenario, trajectory); !errors.Is(err, labspec.ErrInvalid) {
		t.Fatalf("Validate = %v, want ErrInvalid", err)
	}
}

func writeGap(t *testing.T, class, body string) string {
	t.Helper()
	directory := filepath.Join(t.TempDir(), class)
	if err := os.Mkdir(directory, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "stub-01-allow-read.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const gapBlock = `gap:
  wanted: { 3: { verdict: DENY, reason_codes_include: [TOXIC_FLOW_SENSITIVE_TO_EXTERNAL] } }
  why: the gateway builds no run flow
`

func TestAGapLivesInTheGapsSuiteAndNowhereElse(t *testing.T) {
	if _, err := labspec.LoadScenario(writeGap(t, "gaps", goodScenario+gapBlock)); err != nil {
		t.Fatalf("a gap under gaps/ was refused: %v", err)
	}
	for name, path := range map[string]string{
		"a gap outside gaps/":      writeGap(t, "flow", goodScenario+gapBlock),
		"gaps/ without a gap":      writeGap(t, "gaps", goodScenario),
		"a gap with no reason":     writeGap(t, "gaps", goodScenario+strings.Replace(gapBlock, "why: the gateway builds no run flow", "why: ''", 1)),
		"a gap wanting nothing":    writeGap(t, "gaps", goodScenario+"gap: { wanted: {}, why: x }\n"),
		"a gap for a lost step":    writeGap(t, "gaps", goodScenario+strings.Replace(gapBlock, "{ 3:", "{ 9:", 1)),
		"a gap wanting no verdict": writeGap(t, "gaps", goodScenario+strings.Replace(gapBlock, "verdict: DENY, ", "", 1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := labspec.LoadScenario(path); !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestStepTimingIsReadAndBounded(t *testing.T) {
	timed := strings.Replace(goodTrajectory, "  - call:\n      server: victim-fs",
		"  - wait_before: 1500ms\n    retry_while_pending: { every: 2s, at_most: 3 }\n    call:\n      server: victim-fs", 1)
	trajectory, err := labspec.LoadTrajectory(writeFile(t, "flow.yaml", timed))
	if err != nil {
		t.Fatalf("LoadTrajectory: %v", err)
	}
	if got := trajectory.Steps[1].Budget(); got != 7500*time.Millisecond {
		t.Errorf("budget = %s, want 7.5s", got)
	}
	for name, replacement := range map[string]string{
		"a bare number":         "wait_before: 1500",
		"a wait past the bound": "wait_before: 10m1s",
		"a negative wait":       "wait_before: -1s",
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.Replace(timed, "wait_before: 1500ms", replacement, 1)
			if _, err := labspec.LoadTrajectory(writeFile(t, "flow.yaml", body)); !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
	for name, replacement := range map[string]string{
		"a retry too eager":    "{ every: 99ms, at_most: 3 }",
		"a retry too patient":  "{ every: 61s, at_most: 3 }",
		"no retry at all":      "{ every: 2s, at_most: 0 }",
		"too many retries":     "{ every: 2s, at_most: 101 }",
		"retrying past bounds": "{ every: 60s, at_most: 11 }",
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.Replace(timed, "{ every: 2s, at_most: 3 }", replacement, 1)
			if _, err := labspec.LoadTrajectory(writeFile(t, "flow.yaml", body)); !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestOneThingDecidesATrajectoryScenario(t *testing.T) {
	stub := "stub: { verdicts: config/scenarios/stub-01.yaml }\n"
	for name, body := range map[string]string{
		"both":                  strings.Replace(heldScenario, "gateway:\n", stub+"gateway:\n", 1),
		"neither":               strings.Replace(heldScenario, "gateway:\n  config: config/gateway/scenarios/stub-01-allow-read.yaml\n  policy: config/policies/stub-01-allow-read.json\n", "", 1),
		"config elsewhere":      strings.Replace(heldScenario, "config: config/gateway/scenarios/", "config: config/", 1),
		"policy elsewhere":      strings.Replace(heldScenario, "policy: config/policies/", "policy: config/gateway/", 1),
		"a climb":               strings.Replace(heldScenario, "config/policies/stub-01", "config/policies/../../x/stub-01", 1),
		"an unclassified typo":  strings.Replace(heldScenario, "  policy: config/policies/stub-01-allow-read.json\n", "  policy: config/policies/stub-01-allow-read.json\n  unclassified: [victim-shell.shell.exec]\n", 1),
		"an unclassified twice": strings.Replace(heldScenario, "  policy: config/policies/stub-01-allow-read.json\n", "  policy: config/policies/stub-01-allow-read.json\n  unclassified: [victim-shell/shell.exec, victim-shell/shell.exec]\n", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if body == heldScenario {
				t.Fatal("the mutation did not apply")
			}
			if _, err := labspec.LoadScenario(writeFile(t, "stub-01-allow-read.yaml", body)); !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
	withUnclassified := strings.Replace(heldScenario, "  policy: config/policies/stub-01-allow-read.json\n", "  policy: config/policies/stub-01-allow-read.json\n  unclassified: [victim-shell/shell.exec]\n", 1)
	scenario, err := labspec.LoadScenario(writeFile(t, "stub-01-allow-read.yaml", withUnclassified))
	if err != nil || !scenario.UsesEnforcer() || scenario.Gateway.Unclassified[0] != "victim-shell/shell.exec" {
		t.Fatalf("a gateway scenario did not load as one: %+v, %v", scenario.Gateway, err)
	}
}
