package labspec

// VerifierStep is one command the verifier runs. Probe is the only command
// today; the wrapper keeps the list open to others without a new format.
type VerifierStep struct {
	Probe *ProbeStep `json:"probe"`
}

// ProfileDir is where a verifier step's profiles live.
const ProfileDir = "config/verifier/profiles/"

// ProbeStep examines one MCP server of the lab, addressed by the name compose
// gives it. WritePin approves the manifest it reads; PinFrom compares the
// manifest against the pin an earlier step wrote for the same server, PinOf
// against one written for another server, which the verifier cannot use.
// Profile is a verifier profile under ProfileDir, handed to this step alone.
type ProbeStep struct {
	Server   string `json:"server"`
	WritePin bool   `json:"write_pin,omitempty"`
	PinFrom  int    `json:"pin_from,omitempty"`
	PinOf    int    `json:"pin_of,omitempty"`
	Profile  string `json:"profile,omitempty"`
	// Credential hands the step the run's bearer token, by the name of the
	// variable holding it and never by value.
	Credential bool `json:"credential,omitempty"`
}

// VerifierExpectation is what one verifier step has to leave: the exit status,
// and in the report it printed, the findings and unverified results it names,
// the verdict the verifier recorded for its own run, and why. ExitCode is a
// pointer because 0 is an expectation and not an absence; Shortfall and Errors
// are exact lists, so an empty one says there are none and an absent one says
// nothing.
type VerifierExpectation struct {
	ExitCode          *int                    `json:"exit_code"`
	FindingsInclude   []FindingExpectation    `json:"findings_include,omitempty"`
	FindingsExclude   []string                `json:"findings_exclude,omitempty"`
	UnverifiedInclude []string                `json:"unverified_include,omitempty"`
	Gate              string                  `json:"gate,omitempty"`
	Shortfall         *[]ShortfallExpectation `json:"shortfall,omitempty"`
	Errors            *[]string               `json:"errors,omitempty"`
}

// ShortfallExpectation is one coverage gap the report has to record, by the
// verifier's kind and, when stated, what it concerned.
type ShortfallExpectation struct {
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
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
