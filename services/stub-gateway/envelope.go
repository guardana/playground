package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"strconv"
	"time"

	"github.com/guardana/playground/internal/evidence"
)

// What the gateway writes down about one call, and what it reads off the call
// to write it. Nothing here decides anything: the verdict is already in hand.

func (g *gateway) envelope(current *call) *evidence.ActionEnvelope {
	now := time.Now().UTC()
	return &evidence.ActionEnvelope{
		SchemaVersion: wireSchemaVersion,
		RequestID:     current.trail.requestID(),
		OccurredAt:    &now,
		ProjectID:     g.verdicts.ProjectID,
		TenantID:      g.verdicts.TenantID,
		Action:        &evidence.Action{Name: current.tool, Protocol: "mcp"},
		Resource:      &evidence.Resource{Type: "mcp_tool", ID: current.up.name + "/" + current.tool},
		// The hash is over the argument bytes as they arrived and are
		// forwarded, not over a canonical form of them. RedactedPreview stays
		// unset: a scenario asserts that argument text never reaches the trail.
		Arguments: &evidence.Arguments{CanonicalHash: digestOf(current.params.Arguments)},
		Context: &evidence.RunContext{
			// The run is the one this gateway was started for. The caller also
			// stamps a run id on the call, and that is the caller's claim.
			RunID:  g.trail.identity.runID,
			StepID: current.step,
		},
	}
}

func (g *gateway) decision(current *call, declared answer) *evidence.Decision {
	return &evidence.Decision{
		SchemaVersion:      wireSchemaVersion,
		DecisionID:         newID("dec"),
		RequestID:          current.trail.requestID(),
		ActionDigest:       current.digest,
		PolicyBundleDigest: g.verdicts.PolicyBundleDigest,
		Verdict:            declared.verdict,
		ReasonCodes:        declared.reasonCodes,
		Obligations:        declared.obligations,
		// Named so a reader of the trail can see at once that no policy
		// decision point answered this: a file did.
		PdpInstance: g.name,
	}
}

// stepFromMeta reads the trajectory step the caller stamped on the call.
//
// Anything else, a missing key included, is no step at all, which the verdict
// file answers as INDETERMINATE. A step this gateway cannot read must never
// become a step it forwards.
func stepFromMeta(meta map[string]any) (int, bool) {
	switch value := meta[metaStepKey].(type) {
	case float64: // what a JSON number decodes to
		if value != math.Trunc(value) {
			return 0, false
		}
		return inRange(int64(value))
	case int:
		return inRange(int64(value))
	case int64:
		return inRange(value)
	case json.Number:
		number, err := value.Int64()
		if err != nil {
			return 0, false
		}
		return inRange(number)
	default:
		return 0, false
	}
}

func inRange(number int64) (int, bool) {
	if number < 1 || number > maxStep {
		return 0, false
	}
	return int(number), true
}

// stepID is the step as the trail spells it. An unnumbered call carries no
// stepId rather than a zero, which would read as a step somebody numbered.
func stepID(number int, numbered bool) string {
	if !numbered {
		return ""
	}
	return strconv.Itoa(number)
}

// digestOf is a SHA-256 over the parts, separated so two different splits of
// the same bytes do not collide.
func digestOf(parts ...[]byte) string {
	sum := sha256.New()
	for _, part := range parts {
		_, _ = sum.Write(part)
		_, _ = sum.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(sum.Sum(nil))
}
