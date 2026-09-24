package labspec

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// maxVerifierExitCode is the highest code the verifier's exit-status contract
// documents; a scenario expecting another expects something undocumented.
const maxVerifierExitCode = 7

// VerifierProfile is the compose profile that brings up the victims without the
// gateway, the only one a verifier scenario runs in.
const VerifierProfile = "verifier"

// Severities are spelled as the verifier's JSON report spells them.
var Severities = []string{"INFO", "LOW", "MEDIUM", "HIGH", "CRITICAL"}

// serverName is a compose service name. The runner builds the probed URL from
// it, so anything else would put a scenario's text into a URL.
var serverName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// validateVerifier refuses every key a verifier scenario cannot grade: there
// is no trajectory, no enforcer and no trail, so a stated decision, trail
// expectation, mode, tolerance or gap would load and be read by nothing.
func (s Scenario) validateVerifier() error {
	for _, set := range []struct {
		field string
		set   bool
	}{
		{"trajectory", s.Trajectory != ""},
		{"enforcement_mode", s.EnforcementMode != ""},
		{"stub", s.Stub != (Stub{})},
		{"gap", s.Gap != nil},
		{"tolerance", len(s.Tolerance.AllowIndeterminateForSteps) > 0},
		{"expect.decisions", len(s.Expect.Decisions) > 0},
		{"expect.evidence", s.Expect.Evidence != nil},
	} {
		if set.set {
			return fmt.Errorf("%w: %s is set in a verifier scenario, which has nothing to grade it by", ErrInvalid, set.field)
		}
	}
	if !slices.Equal(s.Profile, []string{VerifierProfile}) {
		// Another profile boots the gateway, which lists every victim's tools at
		// startup and spends the listing a drift is read on.
		return fmt.Errorf("%w: profile is %v; a verifier scenario runs in [%s] alone", ErrInvalid, s.Profile, VerifierProfile)
	}
	for i := range s.Verifier {
		if _, graded := s.Expect.Verifier[i+1]; !graded {
			return fmt.Errorf("%w: no expectation for verifier step %d of %d; a step nobody grades is not a step that passed",
				ErrInvalid, i+1, len(s.Verifier))
		}
		if err := s.validateVerifierStep(i + 1); err != nil {
			return err
		}
	}
	for _, number := range sortedInts(s.Expect.Verifier) {
		if number < 1 || number > len(s.Verifier) {
			return fmt.Errorf("%w: expect.verifier names step %d and there are %d", ErrInvalid, number, len(s.Verifier))
		}
	}
	return s.validateProbedEffects()
}

func (s Scenario) validateVerifierStep(number int) error {
	probe := s.Verifier[number-1].Probe
	field := fmt.Sprintf("verifier[%d].probe", number)
	switch {
	case probe == nil:
		return fmt.Errorf("%w: verifier[%d] names no command", ErrInvalid, number)
	case !serverName.MatchString(probe.Server):
		return fmt.Errorf("%w: %s.server is %q, want a compose service name", ErrInvalid, field, probe.Server)
	case probe.WritePin && probe.PinFrom != 0:
		return fmt.Errorf("%w: %s both writes a pin and compares against one", ErrInvalid, field)
	}
	if probe.PinFrom != 0 {
		if err := s.validatePinFrom(number, *probe); err != nil {
			return err
		}
	}
	return s.Expect.Verifier[number].validate(number, probe.WritePin)
}

// validatePinFrom holds a comparison to a pin an earlier step wrote for the
// same server: a pin of another server is a comparison against a manifest
// nobody approved for this one.
func (s Scenario) validatePinFrom(number int, probe ProbeStep) error {
	from := probe.PinFrom
	if from < 1 || from >= number {
		return fmt.Errorf("%w: verifier[%d] reads the pin of step %d, which has not run before it", ErrInvalid, number, from)
	}
	source := s.Verifier[from-1].Probe
	switch {
	case source == nil || !source.WritePin:
		return fmt.Errorf("%w: verifier[%d] reads the pin of step %d, which writes none", ErrInvalid, number, from)
	case source.Server != probe.Server:
		return fmt.Errorf("%w: verifier[%d] probes %s against the pin step %d wrote for %s",
			ErrInvalid, number, probe.Server, from, source.Server)
	}
	return nil
}

func (v VerifierExpectation) validate(number int, writesPin bool) error {
	field := fmt.Sprintf("expect.verifier[%d]", number)
	switch {
	case v.ExitCode == nil:
		return fmt.Errorf("%w: %s states no exit_code", ErrInvalid, field)
	case *v.ExitCode < 0 || *v.ExitCode > maxVerifierExitCode:
		return fmt.Errorf("%w: %s.exit_code is %d, want 0 to %d", ErrInvalid, field, *v.ExitCode, maxVerifierExitCode)
	case writesPin && (len(v.FindingsInclude) > 0 || len(v.FindingsExclude) > 0 || len(v.UnverifiedInclude) > 0):
		return fmt.Errorf("%w: %s writes a pin, which exits without a report to read findings from", ErrInvalid, field)
	}
	return v.validateRules(field)
}

func (v VerifierExpectation) validateRules(field string) error {
	included := make([]string, 0, len(v.FindingsInclude))
	for i, finding := range v.FindingsInclude {
		if err := finding.validate(fmt.Sprintf("%s.findings_include[%d]", field, i)); err != nil {
			return err
		}
		included = append(included, finding.RuleID)
	}
	for _, rule := range slices.Concat(v.FindingsExclude, v.UnverifiedInclude) {
		if strings.TrimSpace(rule) == "" {
			return fmt.Errorf("%w: %s names an empty rule id", ErrInvalid, field)
		}
	}
	// An excluded rule is one that ran and concluded clean, so it cannot also
	// be a wanted finding or an unverified result.
	for _, rule := range v.FindingsExclude {
		if slices.Contains(included, rule) || slices.Contains(v.UnverifiedInclude, rule) {
			return fmt.Errorf("%w: %s both excludes %s and expects it reported", ErrInvalid, field, rule)
		}
	}
	return nil
}

func (f FindingExpectation) validate(field string) error {
	if err := required(field+".rule_id", f.RuleID); err != nil {
		return err
	}
	if f.Severity != "" {
		return oneOf(field+".severity", f.Severity, Severities...)
	}
	return nil
}

// validateProbedEffects holds expect.effects exhaustive over the probed
// servers: the verifier documents that it never calls a tool, and only a
// journal read back says whether that held.
func (s Scenario) validateProbedEffects() error {
	probed := s.Probed()
	for _, server := range probed {
		if _, stated := s.Expect.Effects[server]; !stated {
			return fmt.Errorf("%w: the verifier probes %s and expect.effects does not say what it served", ErrInvalid, server)
		}
	}
	for _, server := range sortedKeys(s.Expect.Effects) {
		if !slices.Contains(probed, server) {
			return fmt.Errorf("%w: expect.effects names %s and no step probes it", ErrInvalid, server)
		}
	}
	return nil
}

// Probed lists the servers the verifier steps probe, sorted and without repeats.
func (s Scenario) Probed() []string {
	var probed []string
	for _, step := range s.Verifier {
		if step.Probe != nil && !slices.Contains(probed, step.Probe.Server) {
			probed = append(probed, step.Probe.Server)
		}
	}
	slices.Sort(probed)
	return probed
}
