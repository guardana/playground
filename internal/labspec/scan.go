package labspec

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

// validateScanStep holds a scan step to a target under ArtifactDir and to an
// exact record: the verifier lists every artifact rule as run even over an
// empty target, so only the files it says it read and every finding with its
// file establish that the planted file was examined.
func (s Scenario) validateScanStep(number int) error {
	scan := s.Verifier[number-1].Scan
	field := fmt.Sprintf("verifier[%d].scan", number)
	switch {
	case !strings.HasPrefix(scan.Target, ArtifactDir) || !cleanRelative(scan.Target) || scan.Target == strings.TrimSuffix(ArtifactDir, "/"):
		return fmt.Errorf("%w: %s.target is %q, want a directory under %s", ErrInvalid, field, scan.Target, ArtifactDir)
	case scan.Profile != "" && !directlyUnder(scan.Profile, ProfileDir, ".yaml"):
		return fmt.Errorf("%w: %s.profile is %q, want a .yaml file directly in %s", ErrInvalid, field, scan.Profile, ProfileDir)
	}
	return s.Expect.Verifier[number].validateScan(number)
}

func (v VerifierExpectation) validateScan(number int) error {
	field := fmt.Sprintf("expect.verifier[%d]", number)
	if err := v.validateScanShape(field); err != nil {
		return err
	}
	if err := validateScope(field, *v.Scope); err != nil {
		return err
	}
	if err := validateScanFindings(field, *v.Findings, *v.Scope); err != nil {
		return err
	}
	if err := v.validateRules(field); err != nil {
		return err
	}
	return v.validateVerdict(field)
}

// validateScanShape refuses a missing or undocumented exit code, a record
// stated in part, and the finding lists an exact one replaces.
func (v VerifierExpectation) validateScanShape(field string) error {
	switch {
	case v.ExitCode == nil:
		return fmt.Errorf("%w: %s states no exit_code", ErrInvalid, field)
	case *v.ExitCode < 0 || *v.ExitCode > maxVerifierExitCode:
		return fmt.Errorf("%w: %s.exit_code is %d, want 0 to %d", ErrInvalid, field, *v.ExitCode, maxVerifierExitCode)
	case v.Gate == "" || v.Shortfall == nil || v.Errors == nil || v.Scope == nil || v.Findings == nil:
		return fmt.Errorf("%w: %s states a scan's record in part; a scan step states gate, shortfall, errors, scope and findings",
			ErrInvalid, field)
	case len(v.FindingsInclude) > 0 || len(v.FindingsExclude) > 0:
		return fmt.Errorf("%w: %s names findings_include or findings_exclude; a scan's findings are one exact list", ErrInvalid, field)
	}
	return nil
}

// validateScanFindings holds every finding to a rule, a severity, a phrase of
// its summary and a file in scope: the list is exact, so a finding stated by
// its rule alone would match the planted one and any other the rule raised.
func validateScanFindings(field string, findings []FindingExpectation, scope []string) error {
	for i, finding := range findings {
		at := fmt.Sprintf("%s.findings[%d]", field, i)
		if err := finding.validate(at); err != nil {
			return err
		}
		if finding.Severity == "" || strings.TrimSpace(finding.SummaryContains) == "" {
			return fmt.Errorf("%w: %s states no severity or summary_contains", ErrInvalid, at)
		}
		if !slices.Contains(scope, finding.File) {
			return fmt.Errorf("%w: %s.file is %q, which is not in scope", ErrInvalid, at, finding.File)
		}
	}
	return nil
}

// validateScope refuses an empty scope, a file named twice and a name that
// leaves the target.
func validateScope(field string, scope []string) error {
	if len(scope) == 0 {
		return fmt.Errorf("%w: %s.scope is empty; a scan of nothing establishes nothing", ErrInvalid, field)
	}
	seen := map[string]bool{}
	for _, file := range scope {
		if seen[file] || !cleanRelative(file) {
			return fmt.Errorf("%w: %s.scope names %q twice or outside the target", ErrInvalid, field, file)
		}
		seen[file] = true
	}
	return nil
}

// cleanRelative is a relative path that names no parent and nothing twice.
func cleanRelative(name string) bool {
	return name != "" && !path.IsAbs(name) && path.Clean(name) == name && !strings.HasPrefix(name, "../") && name != ".."
}
