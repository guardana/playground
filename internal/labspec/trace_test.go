package labspec_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

const (
	traceLine       = "trace: { contract: config/contracts/lab.yaml, ai_system: support-agent }\n"
	traceExpectLine = "  trace: { exit_code: 0, findings_exclude: [contract.lab.never-shell] }\n"
)

func TestATraceComesWithItsContractAndItsExpectation(t *testing.T) {
	traced := strings.Replace(strings.Replace(heldScenario, "profile: [core, enforcer]", "profile: [core, enforcer, trace]", 1),
		"expect:\n", traceLine+"expect:\n"+traceExpectLine, 1)
	if _, err := labspec.LoadScenario(writeFile(t, "stub-01-allow-read.yaml", traced)); err != nil {
		t.Fatalf("a traced scenario was refused: %v", err)
	}
	for name, body := range map[string]string{
		"no expectation":         strings.Replace(traced, traceExpectLine, "", 1),
		"no trace":               strings.Replace(traced, traceLine, "", 1),
		"a contract elsewhere":   strings.Replace(traced, "config/contracts/lab.yaml", "config/lab.yaml", 1),
		"a contract in a subdir": strings.Replace(traced, "config/contracts/lab.yaml", "config/contracts/team-b/lab.yaml", 1),
		"no ai system":           strings.Replace(traced, "ai_system: support-agent", "ai_system: ''", 1),
		"no exit code":           strings.Replace(traced, "exit_code: 0, ", "", 1),
		"an exit code alone":     strings.Replace(traced, ", findings_exclude: [contract.lab.never-shell]", "", 1),
		"no contract rule named": strings.Replace(traced, "contract.lab.never-shell", "guardana.mcp.cache_scope", 1),
		"no trace profile":       strings.Replace(traced, "profile: [core, enforcer, trace]", "profile: [core, enforcer]", 1),
		"a trace profile alone":  strings.Replace(strings.Replace(traced, traceLine, "", 1), traceExpectLine, "", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if body == traced {
				t.Fatal("the mutation did not apply")
			}
			if _, err := labspec.LoadScenario(writeFile(t, "stub-01-allow-read.yaml", body)); !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

// A finding the scenario wants counts as naming a contract rule as much as
// one it excludes, and a refusal names the field a person wrote.
func TestATraceExpectationNamesTheContractItGrades(t *testing.T) {
	traced := strings.Replace(strings.Replace(heldScenario, "profile: [core, enforcer]", "profile: [core, enforcer, trace]", 1),
		"expect:\n", traceLine+"expect:\n", 1)
	wanted := "  trace: { exit_code: 1, findings_include: [ { rule_id: contract.lab.needs-approval, severity: HIGH } ] }\n"
	if _, err := labspec.LoadScenario(writeFile(t, "stub-01-allow-read.yaml", strings.Replace(traced, "expect:\n", "expect:\n"+wanted, 1))); err != nil {
		t.Fatalf("a trace expecting a contract finding was refused: %v", err)
	}
	for _, expectation := range []string{"  trace: { exit_code: 9, findings_exclude: [contract.lab.x] }\n", "  trace: { exit_code: 0 }\n"} {
		_, err := labspec.LoadScenario(writeFile(t, "stub-01-allow-read.yaml", strings.Replace(traced, "expect:\n", "expect:\n"+expectation, 1)))
		if err == nil || !strings.Contains(err.Error(), "expect.trace") || strings.Contains(err.Error(), "expect.verifier") {
			t.Errorf("%s was refused with %v, want an error naming expect.trace", strings.TrimSpace(expectation), err)
		}
	}
}
