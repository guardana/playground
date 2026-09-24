package evidence_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/evidence"
)

// One well formed trail, as protojson writes it: enum names, a timestamp in
// RFC 3339, and a 64-bit integer as a string.
const trail = `{"eventId":"e1","kind":"EVENT_KIND_ACTION_PROPOSED","requestId":"r1","projectId":"p","tenantId":"t","occurredAt":"2026-09-09T10:00:00Z","schemaVersion":"1.0","enforcementMode":"ENFORCEMENT_MODE_ENFORCE","proposed":{"schemaVersion":"1.0","requestId":"r1","action":{"name":"fs.read","protocol":"mcp","effect":"EFFECT_CLASS_READ","provider":"victim-fs"}}}
{"eventId":"e2","kind":"EVENT_KIND_POLICY_DECIDED","requestId":"r1","projectId":"p","tenantId":"t","occurredAt":"2026-09-09T10:00:01Z","schemaVersion":"1.0","enforcementMode":"ENFORCEMENT_MODE_ENFORCE","prevEventId":"e1","decision":{"schemaVersion":"1.0","decisionId":"d1","requestId":"r1","verdict":"VERDICT_ALLOW","reasonCodes":["RULE_ALLOW"],"policyBundleDigest":"sha256:aa","decisionLatencyUs":"1200"}}
{"eventId":"e3","kind":"EVENT_KIND_ACTION_STARTED","requestId":"r1","projectId":"p","tenantId":"t","occurredAt":"2026-09-09T10:00:02Z","schemaVersion":"1.0","enforcementMode":"ENFORCEMENT_MODE_ENFORCE","executionId":"x1","prevEventId":"e2"}
{"eventId":"e4","kind":"EVENT_KIND_ACTION_COMPLETED","requestId":"r1","projectId":"p","tenantId":"t","occurredAt":"2026-09-09T10:00:03Z","schemaVersion":"1.0","enforcementMode":"ENFORCEMENT_MODE_ENFORCE","executionId":"x1","prevEventId":"e3","result":{"schemaVersion":"1.0","requestId":"r1","executionId":"x1","status":"RESULT_STATUS_SUCCESS","resultSchemaValid":true}}
`

func decode(t *testing.T, body string) []evidence.Event {
	t.Helper()
	events, err := evidence.DecodeJSONL(strings.NewReader(body), 64)
	if err != nil {
		t.Fatalf("DecodeJSONL: %v", err)
	}
	return events
}

func TestDecodeReadsTheWireShape(t *testing.T) {
	events := decode(t, trail)
	if len(events) != 4 {
		t.Fatalf("events = %d, want 4", len(events))
	}
	if events[0].Proposed.Action.Effect != "EFFECT_CLASS_READ" {
		t.Errorf("effect = %q", events[0].Proposed.Action.Effect)
	}
	if events[1].Decision.Verdict != "VERDICT_ALLOW" {
		t.Errorf("verdict = %q", events[1].Decision.Verdict)
	}
	if events[1].Decision.DecisionLatencyUs != 1200 {
		t.Errorf("latency = %d, want 1200", events[1].Decision.DecisionLatencyUs)
	}
	if got := events[0].OccurredAt.UTC().Format("15:04:05"); got != "10:00:00" {
		t.Errorf("occurredAt = %q", got)
	}
	if !events[3].Result.ResultSchemaValid {
		t.Error("resultSchemaValid was dropped")
	}
}

// A field this reader has no name for means the producer holds a schema the lab
// does not. Dropping it would grade a run against less than was recorded.
func TestDecodeRefusesAnUnknownField(t *testing.T) {
	body := strings.Replace(trail, `"eventId":"e1"`, `"eventId":"e1","surprise":1`, 1)
	if _, err := evidence.DecodeJSONL(strings.NewReader(body), 64); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestDecodeRefusesABlankLine(t *testing.T) {
	body := strings.Replace(trail, "\n{\"eventId\":\"e2\"", "\n\n{\"eventId\":\"e2\"", 1)
	if _, err := evidence.DecodeJSONL(strings.NewReader(body), 64); !errors.Is(err, evidence.ErrMalformedLine) {
		t.Fatalf("err = %v, want ErrMalformedLine", err)
	}
}

// A limit is never a suggestion: a trail longer than the caller said it would
// hold is refused rather than truncated into a shorter, well formed one.
func TestDecodeRefusesMoreEventsThanTheLimit(t *testing.T) {
	if _, err := evidence.DecodeJSONL(strings.NewReader(trail), 3); !errors.Is(err, evidence.ErrTooManyEvents) {
		t.Fatalf("err = %v, want ErrTooManyEvents", err)
	}
}

func TestDecodeAcceptsAnIntegerWrittenAsANumber(t *testing.T) {
	body := strings.Replace(trail, `"decisionLatencyUs":"1200"`, `"decisionLatencyUs":1200`, 1)
	events := decode(t, body)
	if events[1].Decision.DecisionLatencyUs != 1200 {
		t.Errorf("latency = %d, want 1200", events[1].Decision.DecisionLatencyUs)
	}
}

func TestValidateChainAcceptsTheDocumentedOrder(t *testing.T) {
	if err := evidence.ValidateChain(decode(t, trail)); err != nil {
		t.Fatalf("ValidateChain: %v", err)
	}
}

// An empty trail is the shape a dropped read has. Reporting it as well formed
// is the false green this repository exists to catch.
func TestValidateChainRefusesNoEvents(t *testing.T) {
	if err := evidence.ValidateChain(nil); !errors.Is(err, evidence.ErrChainBroken) {
		t.Fatalf("err = %v, want ErrChainBroken", err)
	}
}

func TestValidateChainRefusesABrokenLink(t *testing.T) {
	body := strings.Replace(trail, `"prevEventId":"e2"`, `"prevEventId":"e1"`, 1)
	if err := evidence.ValidateChain(decode(t, body)); !errors.Is(err, evidence.ErrChainBroken) {
		t.Fatalf("err = %v, want ErrChainBroken", err)
	}
}

func TestValidateChainRefusesARepeatedEventID(t *testing.T) {
	body := strings.Replace(trail, `"eventId":"e3"`, `"eventId":"e2"`, 1)
	if err := evidence.ValidateChain(decode(t, body)); !errors.Is(err, evidence.ErrChainBroken) {
		t.Fatalf("err = %v, want ErrChainBroken", err)
	}
}

func TestValidateChainRefusesAStepTheOrderDoesNotAllow(t *testing.T) {
	// Completed without anything having started.
	body := strings.Replace(trail, `"kind":"EVENT_KIND_ACTION_STARTED"`, `"kind":"EVENT_KIND_ACTION_COMPLETED"`, 1)
	if err := evidence.ValidateChain(decode(t, body)); !errors.Is(err, evidence.ErrChainBroken) {
		t.Fatalf("err = %v, want ErrChainBroken", err)
	}
}

// A kind from a later minor version is neither well formed nor broken to this
// reader. Saying so is what lets the contract add a kind without every older
// consumer either lying or failing.
func TestValidateChainReportsAKindItCannotPlace(t *testing.T) {
	body := strings.Replace(trail, `"kind":"EVENT_KIND_ACTION_STARTED"`, `"kind":"EVENT_KIND_SOMETHING_LATER"`, 1)
	err := evidence.ValidateChain(decode(t, body))
	if !errors.Is(err, evidence.ErrChainIndeterminate) {
		t.Fatalf("err = %v, want ErrChainIndeterminate", err)
	}
}

// A definite defect outranks an undetermined one: a broken link is reported as
// broken even when the sequence also carries a kind this reader cannot place.
func TestValidateChainPrefersADefiniteDefect(t *testing.T) {
	body := strings.Replace(trail, `"kind":"EVENT_KIND_ACTION_STARTED"`, `"kind":"EVENT_KIND_SOMETHING_LATER"`, 1)
	body = strings.Replace(body, `"prevEventId":"e2"`, `"prevEventId":"e1"`, 1)
	if err := evidence.ValidateChain(decode(t, body)); !errors.Is(err, evidence.ErrChainBroken) {
		t.Fatalf("err = %v, want ErrChainBroken", err)
	}
}

func TestValidateChainRefusesTwoRequestsInOneChain(t *testing.T) {
	body := strings.Replace(trail, `"eventId":"e3","kind":"EVENT_KIND_ACTION_STARTED","requestId":"r1"`,
		`"eventId":"e3","kind":"EVENT_KIND_ACTION_STARTED","requestId":"r2"`, 1)
	if err := evidence.ValidateChain(decode(t, body)); !errors.Is(err, evidence.ErrChainBroken) {
		t.Fatalf("err = %v, want ErrChainBroken", err)
	}
}

// request_id is unique only within a project, and a trail is one tenant's, so
// a second project or tenant under one request is a broken trail, not a join.
func TestValidateChainRefusesASecondScopeInOneChain(t *testing.T) {
	for name, scope := range map[string]string{
		"two tenants":   `"projectId":"p","tenantId":"u"`,
		"two projects":  `"projectId":"q","tenantId":"t"`,
		"later no one":  `"projectId":"","tenantId":""`,
		"later omitted": `"projectId":"p"`,
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.Replace(trail, `"eventId":"e2","kind":"EVENT_KIND_POLICY_DECIDED","requestId":"r1","projectId":"p","tenantId":"t"`,
				`"eventId":"e2","kind":"EVENT_KIND_POLICY_DECIDED","requestId":"r1",`+scope, 1)
			if body == trail {
				t.Fatal("fixture unchanged")
			}
			if err := evidence.ValidateChain(decode(t, body)); !errors.Is(err, evidence.ErrChainBroken) {
				t.Fatalf("err = %v, want ErrChainBroken", err)
			}
		})
	}
}

// The first event sets the scope, so an empty one there would let every event
// agree with a trail that names no project or no tenant.
func TestValidateChainRefusesAnUnscopedFirstEvent(t *testing.T) {
	for name, field := range map[string]string{"no project": `"projectId":"p",`, "no tenant": `"tenantId":"t",`} {
		t.Run(name, func(t *testing.T) {
			body := strings.ReplaceAll(trail, field, "")
			if err := evidence.ValidateChain(decode(t, body)); !errors.Is(err, evidence.ErrChainBroken) {
				t.Fatalf("err = %v, want ErrChainBroken", err)
			}
		})
	}
}

// A run holds one trail per request. Grouping keeps each request's events in
// the order the file carried them, because the order is what is being checked.
func TestByRequestKeepsFileOrderPerRequest(t *testing.T) {
	body := trail + strings.ReplaceAll(strings.Replace(trail, `"eventId":"e1"`, `"eventId":"f1"`, 1), `"requestId":"r1"`, `"requestId":"r2"`)
	byRequest := evidence.ByRequest(decode(t, body))
	if len(byRequest) != 2 {
		t.Fatalf("requests = %d, want 2", len(byRequest))
	}
	if got := byRequest["r1"][0].EventID; got != "e1" {
		t.Errorf("first event of r1 = %q, want e1", got)
	}
	if n := len(byRequest["r2"]); n != 4 {
		t.Errorf("r2 has %d events, want 4", n)
	}
}
