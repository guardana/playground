package labspec

import (
	"fmt"
	"slices"
	"strings"
)

// ContractDir is where a scenario's security contracts live.
const ContractDir = "config/contracts/"

// TraceProfile is the compose profile holding the verifier that grades the
// trace; a scenario names it exactly when it names a trace.
const TraceProfile = "trace"

// Trace has the verifier grade the agent's own record of the run, written in
// the verifier's native trace dialect with effects and approvals, against a
// security contract. The verifier reads what a team's instrumentation would
// hand it; the lab grades the verifier's verdict, and the run's journals say
// what really happened.
type Trace struct {
	Contract string `json:"contract"`
	AISystem string `json:"ai_system"`
}

func (s Scenario) validateTrace() error {
	switch {
	case slices.Contains(s.Profile, TraceProfile) != (s.Trace != nil):
		return fmt.Errorf("%w: profile %s and trace go together", ErrInvalid, TraceProfile)
	case s.Trace == nil && s.Expect.Trace == nil:
		return nil
	case s.Trace == nil:
		return fmt.Errorf("%w: expect.trace is set and the scenario names no trace", ErrInvalid)
	case s.Expect.Trace == nil:
		return fmt.Errorf("%w: trace is set and expect.trace says nothing about the verdict", ErrInvalid)
	case !underWith(s.Trace.Contract, ContractDir, ".yaml") || strings.Contains(strings.TrimPrefix(s.Trace.Contract, ContractDir), "/"):
		// The runner hands the verifier the base name under the contracts
		// mount, so a file in a subdirectory would be graded as another one.
		return fmt.Errorf("%w: trace.contract is %q, want a .yaml file directly in %s", ErrInvalid, s.Trace.Contract, ContractDir)
	case strings.TrimSpace(s.Trace.AISystem) == "":
		return fmt.Errorf("%w: trace.ai_system is empty", ErrInvalid)
	}
	return s.Expect.Trace.validateTrace()
}

// validateTrace holds the analysis to naming a contract rule it grades: an exit
// code alone passes on a trace cut short, whose every rule came back
// unverified and none of them concluded.
func (v VerifierExpectation) validateTrace() error {
	const field = "expect.trace"
	named := slices.ContainsFunc(v.FindingsExclude, isContractRule)
	for _, finding := range v.FindingsInclude {
		named = named || isContractRule(finding.RuleID)
	}
	switch {
	case v.ExitCode == nil:
		return fmt.Errorf("%w: %s states no exit_code", ErrInvalid, field)
	case *v.ExitCode < 0 || *v.ExitCode > maxVerifierExitCode:
		return fmt.Errorf("%w: %s.exit_code is %d, want 0 to %d", ErrInvalid, field, *v.ExitCode, maxVerifierExitCode)
	case !named:
		return fmt.Errorf("%w: %s names no %s rule in findings_include or findings_exclude", ErrInvalid, field, contractRulePrefix)
	}
	return v.validateRules(field)
}

// contractRulePrefix starts the rule id of every contract assertion.
const contractRulePrefix = "contract."

func isContractRule(rule string) bool { return strings.HasPrefix(rule, contractRulePrefix) }
