package labspec

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// maxVerifierExitCode is the highest code the verifier's exit-status contract
// documents; a scenario expecting another expects something undocumented.
const maxVerifierExitCode = 8

// VerifierProfile is the compose profile that brings up the victims without the
// gateway, the only one a verifier scenario runs in.
const VerifierProfile = "verifier"

// Severities are spelled as the verifier's JSON report spells them.
var Severities = []string{"INFO", "LOW", "MEDIUM", "HIGH", "CRITICAL"}

// Gates are the verdicts the verifier's report records for its own run.
var Gates = []string{"pass", "fail", "indeterminate"}

// indeterminateExit is the verifier's exit code for a run it could not decide,
// which has as many causes as the report has channels.
const indeterminateExit = 2

// serverName is a compose service name. The runner builds the probed URL from
// it, so anything else would put a scenario's text into a URL.
var serverName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// validateVerifier refuses every key a verifier scenario cannot grade: there
// is no trajectory, no enforcer and no trail, so a gateway, a stated decision,
// trail expectation, mode, tolerance or gap would load and be read by nothing.
func (s Scenario) validateVerifier() error {
	for _, set := range []struct {
		field string
		set   bool
	}{
		{"gateway", s.Gateway != nil},
		{"trajectory", s.Trajectory != ""},
		{"enforcement_mode", s.EnforcementMode != ""},
		{"gap", s.Gap != nil},
		{"tolerance", len(s.Tolerance.AllowIndeterminateForSteps) > 0},
		{"expect.decisions", len(s.Expect.Decisions) > 0},
		{"expect.evidence", s.Expect.Evidence != nil},
		{"trace", s.Trace != nil || s.Expect.Trace != nil},
		{"chaos", len(s.Chaos) > 0},
	} {
		if set.set {
			return fmt.Errorf("%w: %s is set in a verifier scenario, which has nothing to grade it by", ErrInvalid, set.field)
		}
	}
	profile := VerifierProfile
	if s.IsScan() {
		profile = ScanProfile
	}
	if !slices.Equal(s.Profile, []string{profile}) {
		// Another profile boots the gateway, which lists every victim's tools at
		// startup and spends the listing a drift is read on; a scan reaches
		// nothing, so its profile boots nothing.
		return fmt.Errorf("%w: profile is %v; this verifier scenario runs in [%s] alone", ErrInvalid, s.Profile, profile)
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
	step := s.Verifier[number-1]
	switch {
	case step.Probe != nil && step.Scan != nil:
		return fmt.Errorf("%w: verifier[%d] names two commands", ErrInvalid, number)
	case s.IsScan() && step.Scan == nil:
		return fmt.Errorf("%w: verifier[%d] is not a scan, and a scan scenario runs scan steps alone", ErrInvalid, number)
	case step.Scan != nil:
		return s.validateScanStep(number)
	}
	probe := step.Probe
	field := fmt.Sprintf("verifier[%d].probe", number)
	switch {
	case probe == nil:
		return fmt.Errorf("%w: verifier[%d] names no command", ErrInvalid, number)
	case !serverName.MatchString(probe.Server):
		return fmt.Errorf("%w: %s.server is %q, want a compose service name", ErrInvalid, field, probe.Server)
	case countSet(probe.WritePin, probe.PinFrom != 0, probe.PinOf != 0) > 1:
		return fmt.Errorf("%w: %s states more than one of write_pin, pin_from and pin_of", ErrInvalid, field)
	case probe.Profile != "" && !directlyUnder(probe.Profile, ProfileDir, ".yaml"):
		// Only the profile file is handed over, so a file it sits beside
		// would not be there.
		return fmt.Errorf("%w: %s.profile is %q, want a .yaml file directly in %s", ErrInvalid, field, probe.Profile, ProfileDir)
	}
	if err := s.validatePinSource(number, *probe); err != nil {
		return err
	}
	return s.Expect.Verifier[number].validate(number, probe.WritePin)
}

// validatePinSource holds pin_from to a pin an earlier step wrote for the same
// server, and pin_of to one written for another: comparing against a manifest
// nobody approved for this server is a case only when the scenario says so.
func (s Scenario) validatePinSource(number int, probe ProbeStep) error {
	from, same := probe.PinFrom, true
	if probe.PinOf != 0 {
		from, same = probe.PinOf, false
	}
	if from == 0 {
		return nil
	}
	if from < 1 || from >= number {
		return fmt.Errorf("%w: verifier[%d] reads the pin of step %d, which has not run before it", ErrInvalid, number, from)
	}
	source := s.Verifier[from-1].Probe
	switch {
	case source == nil || !source.WritePin:
		return fmt.Errorf("%w: verifier[%d] reads the pin of step %d, which writes none", ErrInvalid, number, from)
	case same && source.Server != probe.Server:
		return fmt.Errorf("%w: verifier[%d] probes %s against the pin step %d wrote for %s; pin_of says that is meant",
			ErrInvalid, number, probe.Server, from, source.Server)
	case !same && source.Server == probe.Server:
		return fmt.Errorf("%w: verifier[%d] states pin_of for the pin step %d wrote for the same %s; that is pin_from",
			ErrInvalid, number, from, probe.Server)
	}
	return nil
}

func countSet(flags ...bool) int {
	set := 0
	for _, flag := range flags {
		if flag {
			set++
		}
	}
	return set
}

// directlyUnder is a file with this suffix in directory itself, never below
// it: the runner hands the verifier the file alone.
func directlyUnder(path, directory, suffix string) bool {
	return underWith(path, directory, suffix) && !strings.Contains(strings.TrimPrefix(path, directory), "/")
}

func (v VerifierExpectation) validate(number int, writesPin bool) error {
	field := fmt.Sprintf("expect.verifier[%d]", number)
	switch {
	case v.ExitCode == nil:
		return fmt.Errorf("%w: %s states no exit_code", ErrInvalid, field)
	case *v.ExitCode < 0 || *v.ExitCode > maxVerifierExitCode:
		return fmt.Errorf("%w: %s.exit_code is %d, want 0 to %d", ErrInvalid, field, *v.ExitCode, maxVerifierExitCode)
	case writesPin && v.readsReport():
		return fmt.Errorf("%w: %s writes a pin, which exits without a report to read findings from", ErrInvalid, field)
	case v.Scope != nil || v.Findings != nil:
		return fmt.Errorf("%w: %s states scope or findings, which only a scan step's report carries", ErrInvalid, field)
	}
	if err := v.validateRules(field); err != nil {
		return err
	}
	return v.validateVerdict(field)
}

func (v VerifierExpectation) readsReport() bool {
	return len(v.FindingsInclude) > 0 || len(v.FindingsExclude) > 0 || len(v.UnverifiedInclude) > 0 ||
		v.Gate != "" || v.Shortfall != nil || v.Errors != nil
}

// validateVerdict refuses a verdict nobody can read and an exit 2 that does
// not say why: the verifier exits 2 for a gap, an error or an unanswered check
// alike, and the lab failing to hand a file over is one of them. The gaps and
// the errors are both stated, because an unverified rule alone is no cause:
// every probe without a token leaves one.
func (v VerifierExpectation) validateVerdict(field string) error {
	if v.Gate != "" {
		if err := oneOf(field+".gate", v.Gate, Gates...); err != nil {
			return err
		}
	}
	for i, gap := range deref(v.Shortfall) {
		if err := required(fmt.Sprintf("%s.shortfall[%d].kind", field, i), gap.Kind); err != nil {
			return err
		}
	}
	for i, source := range deref(v.Errors) {
		if err := required(fmt.Sprintf("%s.errors[%d]", field, i), source); err != nil {
			return err
		}
	}
	if *v.ExitCode != indeterminateExit {
		return nil
	}
	causes := len(deref(v.Shortfall)) + len(deref(v.Errors)) + len(v.UnverifiedInclude)
	if v.Gate == "" || v.Shortfall == nil || v.Errors == nil || causes == 0 {
		return fmt.Errorf("%w: %s expects exit %d without saying why: state gate, shortfall and errors, and a cause in them or in unverified_include",
			ErrInvalid, field, indeterminateExit)
	}
	return nil
}

func deref[T any](list *[]T) []T {
	if list == nil {
		return nil
	}
	return *list
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
