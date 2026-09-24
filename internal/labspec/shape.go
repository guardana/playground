package labspec

import (
	"fmt"
	"slices"
	"strings"
)

// OpensNone is the one value `opens` takes: the step opens no trail of its own.
// At the enforcer's pin neither a retry answered pending nor the retry that
// resumes a hold writes an ACTION_PROPOSED, so the trail cannot tell which
// attempt resumed; the held trail is graded on the step that opened it.
const OpensNone = "none"

// PDPInstanceNone is the `pdp_instance` of a decision that consulted no
// decision point, which the wire carries as an empty field.
const PDPInstanceNone = "none"

// TrailKinds are the event kinds a `trail` list may name, written without the
// EVENT_KIND_ prefix the wire carries.
var TrailKinds = []string{
	"ACTION_PROPOSED", "POLICY_DECIDED", "APPROVAL_REQUESTED", "APPROVAL_DECIDED", "APPROVAL_EXPIRED",
	"ACTION_STARTED", "ACTION_COMPLETED", "ACTION_FAILED", "ACTION_BLOCKED", "FINDING_RAISED", "POLICY_RELOADED",
}

// BlockExpectation is what the trail's ACTION_BLOCKED has to say. A block the
// mode or the plane made is recorded there and not on POLICY_DECIDED, which
// keeps the kernel's own verdict.
type BlockExpectation struct {
	Verdict            string   `json:"verdict"`
	ReasonCodesInclude []string `json:"reason_codes_include,omitempty"`
}

// OpensTrail reports whether the step opens a trail of its own. Trails are
// paired with the steps that open one, in order, because the enforcer writes no
// step number and mints its own request ids.
func (d DecisionExpectation) OpensTrail() bool { return d.Resumes == 0 && d.Opens == "" }

// Graded reports whether the step states anything read from a record.
func (d DecisionExpectation) Graded() bool {
	return d.Verdict != "" || d.Blocked != nil || len(d.Trail) > 0
}

func (d DecisionExpectation) validate(step int) error {
	field := fmt.Sprintf("expect.decisions[%d]", step)
	if err := d.validateShape(field); err != nil {
		return err
	}
	if d.Verdict != "" {
		if err := oneOf(field+".verdict", d.Verdict, Verdicts...); err != nil {
			return err
		}
	}
	if d.Blocked != nil {
		if err := oneOf(field+".blocked.verdict", d.Blocked.Verdict, Verdicts...); err != nil {
			return err
		}
	}
	if d.PDPInstance != "" && d.PDPInstance != PDPInstanceNone && !strings.HasPrefix(d.PDPInstance, "https://") {
		return fmt.Errorf("%w: %s.pdp_instance is %q, want %q or the decision point's https identifier",
			ErrInvalid, field, d.PDPInstance, PDPInstanceNone)
	}
	return validateTrail(field, d)
}

// validateShape refuses a step whose shape leaves nothing to grade, or grades
// what a shape without a record cannot have.
func (d DecisionExpectation) validateShape(field string) error {
	if d.Opens != "" {
		return d.validateOpensNone(field)
	}
	switch {
	case d.Resumes < 0:
		return fmt.Errorf("%w: %s.resumes is %d", ErrInvalid, field, d.Resumes)
	case d.OpensTrail() && d.Verdict == "":
		return fmt.Errorf("%w: %s opens a trail and states no verdict", ErrInvalid, field)
	case d.Resumes == 0:
		return nil
	case d.Verdict != "" || len(d.ReasonCodesInclude) > 0 || len(d.ObligationsInclude) > 0 || d.PDPInstance != "":
		return fmt.Errorf("%w: %s resumes a trail and states a verdict, which only re-reads the opening step's POLICY_DECIDED",
			ErrInvalid, field)
	case d.Blocked == nil && len(d.Trail) == 0:
		return fmt.Errorf("%w: %s resumes a trail and states neither trail nor blocked to read from it", ErrInvalid, field)
	}
	return nil
}

func (d DecisionExpectation) validateOpensNone(field string) error {
	switch {
	case d.Opens != OpensNone:
		return fmt.Errorf("%w: %s.opens is %q, want %q", ErrInvalid, field, d.Opens, OpensNone)
	case d.Resumes != 0:
		return fmt.Errorf("%w: %s both resumes a trail and opens none", ErrInvalid, field)
	case d.Graded() || len(d.ReasonCodesInclude) > 0 || len(d.ObligationsInclude) > 0 || d.PDPInstance != "":
		return fmt.Errorf("%w: %s opens no trail, so there is no record to grade what it states", ErrInvalid, field)
	}
	return nil
}

func validateTrail(field string, d DecisionExpectation) error {
	for i, kind := range d.Trail {
		if err := oneOf(fmt.Sprintf("%s.trail[%d]", field, i), kind, TrailKinds...); err != nil {
			return err
		}
	}
	if len(d.Trail) > 0 && d.Trail[0] != "ACTION_PROPOSED" {
		return fmt.Errorf("%w: %s.trail starts with %s; every trail opens with ACTION_PROPOSED",
			ErrInvalid, field, d.Trail[0])
	}
	if d.Blocked != nil && len(d.Trail) > 0 && !slices.Contains(d.Trail, "ACTION_BLOCKED") {
		return fmt.Errorf("%w: %s.blocked grades an ACTION_BLOCKED its own trail list leaves out", ErrInvalid, field)
	}
	return nil
}

// validateShapes checks what needs the whole scenario: a step resumes only a
// trail an earlier step opened.
func validateShapes(s Scenario, steps int) error {
	for _, number := range sortedInts(s.Expect.Decisions) {
		resumed := s.Expect.Decisions[number].Resumes
		if resumed == 0 {
			continue
		}
		target, stated := s.Expect.Decisions[resumed]
		switch {
		case resumed >= number || resumed > steps:
			return fmt.Errorf("%w: step %d resumes step %d, which has not run before it", ErrInvalid, number, resumed)
		case !stated || !target.OpensTrail():
			return fmt.Errorf("%w: step %d resumes step %d, which opens no trail", ErrInvalid, number, resumed)
		}
	}
	return nil
}

// ShortKind is an event kind without the prefix the wire carries.
func ShortKind(kind string) string { return strings.TrimPrefix(kind, "EVENT_KIND_") }
