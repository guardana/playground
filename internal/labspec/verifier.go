package labspec

// VerifierStep is one command the verifier runs. Probe is the only command
// today; the wrapper keeps the list open to others without a new format.
type VerifierStep struct {
	Probe *ProbeStep `json:"probe"`
}

// ProbeStep examines one MCP server of the lab, addressed by the name compose
// gives it. WritePin approves the manifest it reads; PinFrom compares the
// manifest against the pin an earlier step wrote.
type ProbeStep struct {
	Server   string `json:"server"`
	WritePin bool   `json:"write_pin,omitempty"`
	PinFrom  int    `json:"pin_from,omitempty"`
}

// VerifierExpectation is what one verifier step has to leave: the exit status,
// and in the report it printed, the findings and unverified results it names.
// ExitCode is a pointer because 0 is an expectation and not an absence.
type VerifierExpectation struct {
	ExitCode          *int                 `json:"exit_code"`
	FindingsInclude   []FindingExpectation `json:"findings_include,omitempty"`
	FindingsExclude   []string             `json:"findings_exclude,omitempty"`
	UnverifiedInclude []string             `json:"unverified_include,omitempty"`
}

// FindingExpectation names one finding the report has to carry. Severity is
// spelled as the verifier's JSON spells it; SummaryContains is a text the
// finding's evidence summary contains.
type FindingExpectation struct {
	RuleID          string `json:"rule_id"`
	Severity        string `json:"severity,omitempty"`
	SummaryContains string `json:"summary_contains,omitempty"`
}

// IsVerifier reports whether the scenario runs the verifier rather than a
// trajectory through the enforcer.
func (s Scenario) IsVerifier() bool { return len(s.Verifier) > 0 }
