package labspec_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

const scanScenario = `schema_version: 1
id: verify-01-drift
title: a pickle that imports on load is reported
profile: [scan]
verifier:
  - scan: { target: artifacts/pickle-import/control }
  - scan: { target: artifacts/pickle-import/defect, profile: config/verifier/profiles/strict.yaml }
expect:
  verifier:
    1: { exit_code: 0, gate: pass, shortfall: [], errors: [], scope: [model.pkl], findings: [] }
    2:
      exit_code: 1
      gate: fail
      shortfall: []
      errors: []
      scope: [model.pkl]
      findings:
        - { rule_id: lab.example.pickle_opcode, severity: CRITICAL, file: model.pkl, summary_contains: clab_never_a_module }
`

func TestAScanOfAnArtifactLoadsWithItsExactRecord(t *testing.T) {
	scenario, err := loadVerifier(t, scanScenario)
	if err != nil {
		t.Fatalf("LoadScenario: %v", err)
	}
	if !scenario.IsScan() || scenario.Verifier[1].Scan == nil || scenario.Verifier[1].Scan.Target != "artifacts/pickle-import/defect" {
		t.Fatalf("step 2 = %+v", scenario.Verifier[1])
	}
	second := scenario.Expect.Verifier[2]
	if second.Scope == nil || len(*second.Scope) != 1 || second.Findings == nil || len(*second.Findings) != 1 ||
		(*second.Findings)[0].File != "model.pkl" {
		t.Errorf("expect.verifier[2] = %+v", second)
	}
	if first := scenario.Expect.Verifier[1]; first.Findings == nil || len(*first.Findings) != 0 {
		t.Errorf("findings: [] was not read as an exact empty list: %+v", first.Findings)
	}
}

func TestAScanThatCouldPassOnNothingIsRefused(t *testing.T) {
	for name, replace := range map[string][]string{
		"a scan in the verifier profile": {"profile: [scan]", "profile: [verifier]"},
		"a probe beside a scan": {"  - scan: { target: artifacts/pickle-import/control }",
			"  - probe: { server: victim-fs }"},
		"a target outside artifacts":   {"artifacts/pickle-import/control }", "attacks/pickle-import/control }"},
		"a target climbing out":        {"artifacts/pickle-import/control }", "artifacts/../config }"},
		"no scope stated":              {"errors: [], scope: [model.pkl], findings: [] }", "errors: [], findings: [] }"},
		"no findings stated":           {"errors: [], scope: [model.pkl], findings: [] }", "errors: [], scope: [model.pkl] }"},
		"no gate stated":               {"    1: { exit_code: 0, gate: pass,", "    1: { exit_code: 0,"},
		"no shortfall stated":          {"    1: { exit_code: 0, gate: pass, shortfall: [],", "    1: { exit_code: 0, gate: pass,"},
		"no errors stated":             {"shortfall: [], errors: [], scope: [model.pkl], findings: [] }", "shortfall: [], scope: [model.pkl], findings: [] }"},
		"an undocumented exit code":    {"    1: { exit_code: 0, gate: pass,", "    1: { exit_code: 9, gate: pass,"},
		"a profile elsewhere":          {"profile: config/verifier/profiles/strict.yaml }", "profile: config/contracts/strict.yaml }"},
		"a finding of no severity":     {"severity: CRITICAL, file:", "file:"},
		"a finding of no summary":      {", summary_contains: clab_never_a_module }", " }"},
		"a finding of no file":         {"file: model.pkl, ", ""},
		"a rule excluded":              {"scope: [model.pkl], findings: [] }", "scope: [model.pkl], findings: [], findings_exclude: [x] }"},
		"a finding included":           {"scope: [model.pkl], findings: [] }", "scope: [model.pkl], findings: [], findings_include: [{ rule_id: x }] }"},
		"an empty scope":               {"errors: [], scope: [model.pkl], findings: [] }", "errors: [], scope: [], findings: [] }"},
		"a file named twice in scope":  {"      scope: [model.pkl]\n", "      scope: [model.pkl, model.pkl]\n"},
		"a scope file climbing out":    {"      scope: [model.pkl]\n", "      scope: [../model.pkl]\n"},
		"effects for a scan":           {"expect:\n", "expect:\n  effects: { victim-fs: { calls_served: {} } }\n"},
		"a scan with a credential key": {"artifacts/pickle-import/control }", "artifacts/pickle-import/control, credential: true }"},
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.NewReplacer(replace...).Replace(scanScenario)
			if body == scanScenario {
				t.Fatal("the replacement changed nothing")
			}
			if _, err := loadVerifier(t, body); !errors.Is(err, labspec.ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}
