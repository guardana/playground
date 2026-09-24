package main

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
)

// errInvalidVerdicts reports a file this stub will not replay. The message
// names the field, because the reader of it is holding the file.
var errInvalidVerdicts = errors.New("stub-gateway: invalid verdict file")

// verdictSchemaVersion is the only version this stub reads.
const verdictSchemaVersion = 1

// maxVerdictFileBytes bounds one verdict file. They are written by hand and
// read whole.
const maxVerdictFileBytes = 256 << 10

// verdicts is a scenario author's declaration of what the enforcement plane
// would have answered, step by step. Nothing here is decided; every field is
// read back out as it was written.
type verdicts struct {
	SchemaVersion      int            `json:"schema_version"`
	ProjectID          string         `json:"project_id"`
	TenantID           string         `json:"tenant_id"`
	PolicyBundleDigest string         `json:"policy_bundle_digest"`
	Steps              []declaredStep `json:"steps"`
}

// declaredStep is one step's declared answer, in trajectory order from 1.
type declaredStep struct {
	Verdict     string               `json:"verdict"`
	ReasonCodes []string             `json:"reason_codes,omitempty"`
	Obligations []declaredObligation `json:"obligations,omitempty"`
}

type declaredObligation struct {
	Type   string            `json:"type"`
	Params map[string]string `json:"params,omitempty"`
}

// answer is one step's declaration in the spellings the wire uses.
type answer struct {
	verdict     string
	reasonCodes []string
	obligations []evidence.Obligation
}

// forwards reports whether the call reaches its upstream. The two allowing
// verdicts forward; every other answer, INDETERMINATE included, does not.
func (a answer) forwards() bool {
	return a.verdict == wireVerdict("ALLOW") || a.verdict == wireVerdict("ALLOW_WITH_OBLIGATIONS")
}

// NoDeclaredVerdict is the reason code for a step nobody declared an answer
// for. It is deliberately not a code from the enforcement plane's registry:
// this is the stub saying it has nothing to replay, which is a fact about the
// lab and never a decision a policy plane reached.
//
// Keeping it distinct is what stops a scenario passing on silence. A scenario
// that declares INDETERMINATE with a real code, POLICY_UNAVAILABLE among them,
// now produces a different trail from one whose verdict file simply ran out, so
// deleting the declaration turns the run red instead of leaving it green.
const NoDeclaredVerdict = "STUB_NO_DECLARED_VERDICT"

// at reads the answer declared for a step, numbered from 1.
//
// A step nobody declared an answer for is INDETERMINATE with
// NoDeclaredVerdict, and never an allow: a trajectory longer than its verdict
// file has run past the end of what anyone stated, and the honest record of
// that is what stops it passing.
func (v *verdicts) at(step int) answer {
	if step < 1 || step > len(v.Steps) {
		return answer{
			verdict:     wireVerdict("INDETERMINATE"),
			reasonCodes: []string{NoDeclaredVerdict},
		}
	}
	declared := v.Steps[step-1]
	return answer{
		verdict:     wireVerdict(declared.Verdict),
		reasonCodes: declared.ReasonCodes,
		obligations: obligations(declared.Obligations),
	}
}

func obligations(declared []declaredObligation) []evidence.Obligation {
	if len(declared) == 0 {
		return nil
	}
	out := make([]evidence.Obligation, 0, len(declared))
	for _, item := range declared {
		out = append(out, evidence.Obligation{Type: item.Type, Params: item.Params})
	}
	return out
}

// wireVerdict is the enum name for a verdict a scenario file spells without its
// prefix.
func wireVerdict(short string) string { return "VERDICT_" + short }

// loadVerdicts reads one declared verdict file.
//
// Strict: a key this stub has no field for is refused rather than dropped,
// because a misspelled verdict that loads is a step the scenario thinks it
// declared and nobody did.
func loadVerdicts(path string) (*verdicts, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxVerdictFileBytes {
		return nil, fmt.Errorf("%s: %w: %d bytes, limit %d", path, errInvalidVerdicts, info.Size(), maxVerdictFileBytes)
	}
	body, err := os.ReadFile(path) // #nosec G304 -- the path is the file the runner mounted.
	if err != nil {
		return nil, err
	}
	var declared verdicts
	if err := yaml.UnmarshalStrict(body, &declared); err != nil {
		return nil, fmt.Errorf("%s: %w: %w", path, errInvalidVerdicts, err)
	}
	if err := declared.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &declared, nil
}

func (v *verdicts) validate() error {
	if v.SchemaVersion != verdictSchemaVersion {
		return fmt.Errorf("%w: schema_version is %d, want %d", errInvalidVerdicts, v.SchemaVersion, verdictSchemaVersion)
	}
	for field, value := range map[string]string{
		"project_id":           v.ProjectID,
		"tenant_id":            v.TenantID,
		"policy_bundle_digest": v.PolicyBundleDigest,
	} {
		if strings.TrimSpace(value) == "" {
			// The digest is required for the same reason as the identifiers: a
			// scenario asserts it is present in the trail, and a stub that
			// wrote an empty one would fail that assertion at the far end,
			// where the file that caused it is no longer in front of anyone.
			return fmt.Errorf("%w: %s is empty", errInvalidVerdicts, field)
		}
	}
	for number, step := range v.Steps {
		// Verdicts are a closed set the wire contract declares; reason codes
		// are not, and are copied through untouched.
		if !slices.Contains(labspec.Verdicts, step.Verdict) {
			return fmt.Errorf("%w: steps[%d].verdict is %q, want one of %s",
				errInvalidVerdicts, number+1, step.Verdict, strings.Join(labspec.Verdicts, ", "))
		}
		if reason, cannot := unaccountable(step.Verdict); cannot {
			return fmt.Errorf("%w: steps[%d].verdict is %q: %s", errInvalidVerdicts, number+1, step.Verdict, reason)
		}
	}
	return nil
}

// unaccountable reports a verdict this stub cannot write a truthful trail for,
// and why, in words the person holding the file can act on.
//
// It is narrower than the wire contract's set on purpose. The trail is the only
// thing a scenario grades, so a verdict the stub can replay but not account for
// is worse than one it refuses: refusing is a red run with a reason, while
// recording it is a green run over a record that contradicts itself.
func unaccountable(verdict string) (string, bool) {
	switch verdict {
	case "REQUIRE_APPROVAL":
		// The truthful trail for it carries APPROVAL_REQUESTED and then the
		// answer or the expiry. This stub has no approver to ask, so it would
		// write POLICY_DECIDED then ACTION_BLOCKED: an approval required, never
		// requested, and blocked outright. ValidateChain accepts that shape and
		// the decisions check compares only the verdict string, so both would
		// pass over it. Emitting APPROVAL_REQUESTED then APPROVAL_EXPIRED is the
		// larger correct fix and belongs with the plane that can ask somebody.
		return "the stub has no approver, so it would record an approval that was required and never requested", true
	default:
		return "", false
	}
}
