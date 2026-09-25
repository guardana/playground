package labspec

import (
	"fmt"
	"slices"
)

// Verdicts are the five the wire contract declares, written without the
// VERDICT_ prefix the enum carries. A scenario names an outcome; the runner is
// what knows how that outcome is spelled on the wire.
var Verdicts = []string{"ALLOW", "DENY", "REQUIRE_APPROVAL", "ALLOW_WITH_OBLIGATIONS", "INDETERMINATE"}

// EnforcementModes are the six the wire contract declares, lowercased.
var EnforcementModes = []string{"observe", "shadow", "warn", "approve", "enforce", "lockdown"}

// Scenario is what the lab expects one trajectory to produce. Every field under
// Expect is read from a record: a decision, an evidence event, or the victim's
// own journal of what it served. Nothing here reads what the agent said.
type Scenario struct {
	SchemaVersion   int            `json:"schema_version"`
	ID              string         `json:"id"`
	Title           string         `json:"title"`
	MapsTo          MapsTo         `json:"maps_to,omitempty"`
	Profile         []string       `json:"profile"`
	EnforcementMode string         `json:"enforcement_mode"`
	Trajectory      string         `json:"trajectory,omitempty"`
	Verifier        []VerifierStep `json:"verifier,omitempty"`
	Gateway         *Gateway       `json:"gateway,omitempty"`
	Trace           *Trace         `json:"trace,omitempty"`
	Chaos           []Fault        `json:"chaos,omitempty"`
	Gap             *Gap           `json:"gap,omitempty"`
	Expect          Expect         `json:"expect"`
	Tolerance       Tolerance      `json:"tolerance,omitempty"`
}

// MapsTo records which catalogued failure this scenario is an instance of, so a
// reader can go from a red run to the class of problem it belongs to.
type MapsTo struct {
	FailureCatalog []string `json:"failure_catalog,omitempty"`
	OWASPASI       []string `json:"owasp_asi,omitempty"`
}

// Expect holds the three places a run is graded from, and none of them is the
// agent's account of its own work.
type Expect struct {
	Decisions map[int]DecisionExpectation  `json:"decisions,omitempty"`
	Effects   map[string]EffectExpectation `json:"effects"`
	Evidence  *EvidenceExpectation         `json:"evidence,omitempty"`
	// Verifier grades a verifier scenario's steps, which make no call through
	// the enforcer and so leave no decision or trail to grade.
	Verifier map[int]VerifierExpectation `json:"verifier,omitempty"`
	// Trace grades the verifier's analysis of the agent's own trace.
	Trace *VerifierExpectation `json:"trace,omitempty"`
}

// DecisionExpectation is what one step's decision has to say. The reason codes
// and obligations are checked for inclusion rather than equality: a decision
// may carry more than the scenario names, and naming all of them would make
// every scenario a copy of the policy.
//
// A step opens a trail of its own unless it resumes an earlier step's held
// trail or opens none. Verdict and its codes grade POLICY_DECIDED on the
// step's own trail, Blocked grades ACTION_BLOCKED, and Trail is the exact
// sequence of kinds the trail holds when the run ends. A resuming step states
// Trail or Blocked only: the held trail's POLICY_DECIDED is the opening step's.
// PDPInstance grades the decision point POLICY_DECIDED names: an identifier,
// or PDPInstanceNone for a decision that consulted none.
type DecisionExpectation struct {
	Verdict            string            `json:"verdict,omitempty"`
	ReasonCodesInclude []string          `json:"reason_codes_include,omitempty"`
	ObligationsInclude []string          `json:"obligations_include,omitempty"`
	PDPInstance        string            `json:"pdp_instance,omitempty"`
	Resumes            int               `json:"resumes,omitempty"`
	Opens              string            `json:"opens,omitempty"`
	Blocked            *BlockExpectation `json:"blocked,omitempty"`
	Trail              []string          `json:"trail,omitempty"`
}

// EffectExpectation is what one victim served and refused, read back from the
// journal that victim wrote. Both maps are exhaustive: a journal line of this
// run whose status and tool the maps do not name is a call the scenario did not
// expect, and it fails the run. An empty CallsServed is the assertion that the
// victim served nothing, which is what a working DENY looks like from the far
// side of the gateway; an absent CallsRefused is the assertion that it refused
// nothing.
type EffectExpectation struct {
	CallsServed  map[string]int `json:"calls_served"`
	CallsRefused map[string]int `json:"calls_refused,omitempty"`
}

// EvidenceExpectation is what the trail itself has to show, separately from
// what it says about any one step.
type EvidenceExpectation struct {
	ChainComplete       bool `json:"chain_complete"`
	PolicyDigestPresent bool `json:"policy_digest_present"`
	// Whether argument or result text is expected in the trail. False is the
	// assertion that the privacy default held, not the absence of an
	// assertion.
	ContentCaptured bool `json:"content_captured"`
}

// Tolerance lists the steps where INDETERMINATE is an acceptable answer to a
// question the scenario expects a verdict for. It is written out step by step
// because an INDETERMINATE nobody listed is a failure, and a blanket tolerance
// would turn every unanswered call into a pass.
type Tolerance struct {
	AllowIndeterminateForSteps []int `json:"allow_indeterminate_for_steps,omitempty"`
}

func (s Scenario) validate(fileName string) error {
	if s.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: schema_version is %d, want %d", ErrInvalid, s.SchemaVersion, SchemaVersion)
	}
	if err := first(required("id", s.ID), required("title", s.Title)); err != nil {
		return err
	}
	if s.ID != fileName {
		return fmt.Errorf("%w: id is %q and the file is named %q", ErrInvalid, s.ID, fileName)
	}
	if len(s.Profile) == 0 {
		return fmt.Errorf("%w: profile is empty", ErrInvalid)
	}
	if err := s.validateEffectCounts(); err != nil {
		return err
	}
	if s.IsVerifier() {
		return s.validateVerifier()
	}
	return s.validateTrajectoryKind()
}

func (s Scenario) validateTrajectoryKind() error {
	if err := first(
		required("trajectory", s.Trajectory),
		oneOf("enforcement_mode", s.EnforcementMode, EnforcementModes...),
	); err != nil {
		return err
	}
	if s.Expect.Evidence == nil {
		return fmt.Errorf("%w: expect.evidence is missing, so nothing states what the trail has to show", ErrInvalid)
	}
	if err := first(s.validateDecider(), s.validateTrace(), s.validateChaos()); err != nil {
		return err
	}
	if len(s.Expect.Verifier) > 0 {
		return fmt.Errorf("%w: expect.verifier is set and the scenario has no verifier steps", ErrInvalid)
	}
	if len(s.Expect.Decisions) == 0 {
		// Nothing to read a verdict from is nothing to grade the run by.
		return fmt.Errorf("%w: expect.decisions is empty", ErrInvalid)
	}
	for _, number := range sortedInts(s.Expect.Decisions) {
		if err := s.Expect.Decisions[number].validate(number); err != nil {
			return err
		}
	}
	return nil
}

// Validate holds the rules that need both files. They are the rules that stop a
// run reporting green on something nobody looked at: every step graded, every
// server the trajectory touches accounted for, and every tolerance pointing at
// a step where it changes the answer.
func Validate(s Scenario, t Trajectory) error {
	for number := 1; number <= len(t.Steps); number++ {
		if _, covered := s.Expect.Decisions[number]; !covered {
			return fmt.Errorf("%w: no expectation for step %d of %d; a step nobody grades is not a step that passed",
				ErrInvalid, number, len(t.Steps))
		}
	}
	for _, number := range sortedInts(s.Expect.Decisions) {
		if number < 1 || number > len(t.Steps) {
			return fmt.Errorf("%w: expect.decisions names step %d and the trajectory has %d",
				ErrInvalid, number, len(t.Steps))
		}
	}
	if err := validateShapes(s, len(t.Steps)); err != nil {
		return err
	}
	if err := first(validateEffects(s, t), validateTracedSteps(s, t)); err != nil {
		return err
	}
	return validateTolerance(s, t)
}

func validateTolerance(s Scenario, t Trajectory) error {
	for _, number := range s.Tolerance.AllowIndeterminateForSteps {
		if number < 1 || number > len(t.Steps) {
			return fmt.Errorf("%w: tolerance names step %d and the trajectory has %d",
				ErrInvalid, number, len(t.Steps))
		}
		switch s.Expect.Decisions[number].Verdict {
		case "INDETERMINATE":
			return fmt.Errorf("%w: tolerance names step %d, where INDETERMINATE is already the expectation",
				ErrInvalid, number)
		case "":
			return fmt.Errorf("%w: tolerance names step %d, which states no verdict to tolerate", ErrInvalid, number)
		}
		if s.Expect.Decisions[number].PDPInstance != "" {
			return fmt.Errorf("%w: tolerance names step %d, whose pdp_instance a tolerated INDETERMINATE would leave ungraded",
				ErrInvalid, number)
		}
	}
	return nil
}

// servers lists the servers the trajectory calls, sorted and without repeats.
func servers(t Trajectory) []string {
	var names []string
	for _, step := range t.Steps {
		if !slices.Contains(names, step.Call.Server) {
			names = append(names, step.Call.Server)
		}
	}
	slices.Sort(names)
	return names
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func sortedInts[V any](m map[int]V) []int {
	keys := make([]int, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
