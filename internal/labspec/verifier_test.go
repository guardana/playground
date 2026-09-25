package labspec_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

const verifierScenario = `schema_version: 1
id: verify-01-drift
title: a pinned manifest that changes is reported as drift
profile: [verifier]
verifier:
  - probe: { server: victim-fs, write_pin: true }
  - probe: { server: victim-fs, pin_from: 1 }
expect:
  verifier:
    1: { exit_code: 0 }
    2:
      exit_code: 1
      findings_include:
        - { rule_id: guardana.agent.mcp_server_manifest, summary_contains: "'fs.read'" }
        - { rule_id: guardana.mcp.unauthenticated_access, severity: LOW }
      findings_exclude: [guardana.mcp.cache_scope]
      unverified_include: [guardana.mcp.session_binding]
  effects:
    victim-fs: { calls_served: {} }
`

func loadVerifier(t *testing.T, body string) (labspec.Scenario, error) {
	t.Helper()
	return labspec.LoadScenario(writeFile(t, "verify-01-drift.yaml", body))
}

func TestAVerifierScenarioLoadsWithoutATrajectory(t *testing.T) {
	scenario, err := loadVerifier(t, verifierScenario)
	if err != nil {
		t.Fatalf("LoadScenario: %v", err)
	}
	if !scenario.IsVerifier() {
		t.Fatal("a scenario with verifier steps is not a verifier scenario")
	}
	second := scenario.Verifier[1].Probe
	if second.Server != "victim-fs" || second.PinFrom != 1 || second.WritePin {
		t.Errorf("step 2 = %+v", second)
	}
	want := scenario.Expect.Verifier[2]
	if want.ExitCode == nil || *want.ExitCode != 1 || len(want.FindingsInclude) != 2 {
		t.Errorf("expect.verifier[2] = %+v", want)
	}
	if got := want.FindingsInclude[1].Severity; got != "LOW" {
		t.Errorf("severity = %q, want LOW", got)
	}
	if first := scenario.Expect.Verifier[1].ExitCode; first == nil || *first != 0 {
		t.Errorf("an exit code of 0 was not read as stated: %v", first)
	}
}

func TestAVerifierScenarioThatGradesNothingOrContradictsItselfIsRefused(t *testing.T) {
	for name, replace := range map[string][]string{
		"an unknown key":            {"write_pin: true }", "write_pin: true, pin: yes }"},
		"a step nobody grades":      {"    1: { exit_code: 0 }\n", ""},
		"a grade for no step":       {"    1: { exit_code: 0 }\n", "    1: { exit_code: 0 }\n    3: { exit_code: 0 }\n"},
		"a step with no exit code":  {"    1: { exit_code: 0 }", "    1: {}"},
		"an undocumented exit code": {"      exit_code: 1\n", "      exit_code: 9\n"},
		"a step with no command":    {"  - probe: { server: victim-fs, write_pin: true }", "  - {}"},
		"a probe of no server":      {"{ server: victim-fs, write_pin: true }", "{ server: '', write_pin: true }"},
		"a server that is a URL":    {"{ server: victim-fs, write_pin: true }", "{ server: 'http://x/', write_pin: true }"},
		"a pin nobody wrote":        {"write_pin: true }", "write_pin: false }"},
		"a pin from a later step":   {"pin_from: 1 }", "pin_from: 2 }"},
		"a pin from no step":        {"pin_from: 1 }", "pin_from: -1 }"},
		"a pin of another server": {"- probe: { server: victim-fs, write_pin: true }", "- probe: { server: victim-crm, write_pin: true }",
			"    victim-fs: { calls_served: {} }\n", "    victim-fs: { calls_served: {} }\n    victim-crm: { calls_served: {} }\n"},
		"writing and reading a pin":  {"pin_from: 1 }", "pin_from: 1, write_pin: true }"},
		"a pin step grading reports": {"    1: { exit_code: 0 }", "    1: { exit_code: 0, unverified_include: [x] }"},
		"a finding with no rule":     {"severity: LOW }", "severity: LOW, rule_id: '' }"},
		"an unknown severity":        {"severity: LOW }", "severity: low }"},
		"a rule wanted and excluded": {"[guardana.mcp.cache_scope]", "[guardana.mcp.unauthenticated_access]"},
		"excluded and unverified":    {"[guardana.mcp.cache_scope]", "[guardana.mcp.session_binding]"},
		"an empty rule excluded":     {"[guardana.mcp.cache_scope]", "['']"},
		"a probed server unstated":   {"    victim-fs: { calls_served: {} }\n", "    victim-crm: { calls_served: {} }\n"},
		"a profile beside verifier":  {"profile: [verifier]", "profile: [verifier, core]"},
		"another profile":            {"profile: [verifier]", "profile: [core, enforcer]"},
		"a trajectory beside it":     {"profile: [verifier]", "profile: [verifier]\ntrajectory: trajectories/x.yaml"},
		"an enforcement mode":        {"profile: [verifier]", "profile: [verifier]\nenforcement_mode: enforce"},
		"a tolerance":                {"profile: [verifier]", "profile: [verifier]\ntolerance: { allow_indeterminate_for_steps: [1] }"},
		"decisions to grade":         {"  verifier:\n    1:", "  decisions: { 1: { verdict: ALLOW } }\n  verifier:\n    1:"},
		"an evidence expectation":    {"  verifier:\n    1:", "  evidence: { chain_complete: false }\n  verifier:\n    1:"},
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.NewReplacer(replace...).Replace(verifierScenario)
			if body == verifierScenario {
				t.Fatal("the replacement changed nothing")
			}
			if _, err := loadVerifier(t, body); !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestAnUngradedVerifierStepIsNamed(t *testing.T) {
	_, err := loadVerifier(t, strings.Replace(verifierScenario, "    1: { exit_code: 0 }\n", "", 1))
	if !errors.Is(err, labspec.ErrInvalid) || !strings.Contains(err.Error(), "no expectation for verifier step 1") {
		t.Fatalf("err = %v, want the ungraded step named", err)
	}
}

func TestAVerifierScenarioIsNeverAGap(t *testing.T) {
	body := verifierScenario + "gap: { wanted: { 1: { verdict: DENY } }, why: x }\n"
	if _, err := labspec.LoadScenario(writeGap(t, "gaps", strings.Replace(body, "verify-01-drift", "case-01-allow-read", 1))); !errors.Is(err, labspec.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestATrajectoryScenarioStatesItsEvidenceAndNoVerifierGrades(t *testing.T) {
	for name, body := range map[string]string{
		"no evidence": strings.Replace(goodScenario,
			"  evidence:\n    chain_complete: true\n    policy_digest_present: true\n    content_captured: false\n", "", 1),
		"verifier grades": strings.Replace(goodScenario, "  effects:", "  verifier: { 1: { exit_code: 0 } }\n  effects:", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if body == goodScenario {
				t.Fatal("the replacement changed nothing")
			}
			if _, err := labspec.LoadScenario(writeFile(t, "case-01-allow-read.yaml", body)); !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}
