package evidence_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/evidence"
)

// Each line sets every field of api/proto/guardana/control/v1 that one payload
// of Event can carry, spelled from the .proto at the pin and not from the
// mirror. 64-bit integers are JSON numbers here only so the round trip below
// can compare them; the string spelling is covered in evidence_test.go.
const eventHead = `"eventId":"e1","kind":"EVENT_KIND_ACTION_PROPOSED","requestId":"r1","runId":"run1","projectId":"p1","tenantId":"t1","occurredAt":"2026-09-19T10:00:00Z","schemaVersion":"1.0","enforcementMode":"ENFORCEMENT_MODE_ENFORCE","executionId":"x1","prevEventId":"e0","prevEventDigest":"sha256:00"`

var everyFieldLines = map[string]string{
	"proposed": `{` + eventHead + `,"proposed":{"schemaVersion":"1.0","requestId":"r1","traceId":"tr","spanId":"sp","occurredAt":"2026-09-19T10:00:00Z","projectId":"p1","tenantId":"t1","environment":"prod",` +
		`"principal":{"id":"u1","type":"user","authnStrength":"mfa","tenantId":"t1","attributes":{"team":"a"}},` +
		`"agent":{"id":"ag","instanceId":"i1","framework":"fw","version":"1","modelRef":"m"},` +
		`"delegation":[{"from":"u1","to":"ag","scopes":["s"],"reason":"why","issuedAt":"2026-09-19T10:00:00Z","expiresAt":"2026-09-19T11:00:00Z"}],` +
		`"action":{"kind":"tool_call","name":"fs.read","protocol":"mcp","effect":"EFFECT_CLASS_READ","provider":"victim-fs"},` +
		`"resource":{"type":"file","id":"f1","tenantId":"t2","environment":"prod","labels":{"k":"v"}},` +
		`"destination":{"trustZone":"TRUST_ZONE_TRUSTED_INTERNAL","host":"h"},` +
		`"data":{"sensitivities":["SENSITIVITY_CONFIDENTIAL"],"sources":["src"],"containsSecrets":true},` +
		`"arguments":{"canonicalHash":"sha256:11","redactedPreview":"{}","schemaRef":"ref","redactionProfile":"default"},` +
		`"context":{"sessionId":"s1","runId":"run1","stepId":"st1","risk":"low","budgets":{"calls":3},"tags":["client:x"]}}}`,
	"decision": `{` + eventHead + `,"decision":{"schemaVersion":"1.0","decisionId":"d1","requestId":"r1","actionDigest":"sha256:22","policyBundleDigest":"sha256:33","policyRuleIds":["rule1"],` +
		`"verdict":"VERDICT_ALLOW_WITH_OBLIGATIONS","reasonCodes":["RULE_ALLOW"],"obligations":[{"type":"cap_amount","params":{"max":"10"},"advisory":true}],` +
		`"expiresAt":"2026-09-19T11:00:00Z","decisionLatencyUs":1200,"pdpType":"kernel","pdpInstance":"k1","pdpVersion":"1","enforcementMode":"ENFORCEMENT_MODE_ENFORCE",` +
		`"policyFreshness":"POLICY_FRESHNESS_FRESH","policyLoadedAt":"2026-09-19T09:00:00Z","decidedAt":"2026-09-19T10:00:01Z"}}`,
	"approval": `{` + eventHead + `,"approval":{"schemaVersion":"1.0","approvalId":"a1","requestId":"r1","actionDigest":"sha256:22","policyBundleDigest":"sha256:33","state":"APPROVAL_STATE_APPROVED",` +
		`"approverId":"u2","reason":"ok","requestedAt":"2026-09-19T10:00:00Z","decidedAt":"2026-09-19T10:01:00Z","expiresAt":"2026-09-19T11:00:00Z","multiUse":true}}`,
	"result": `{` + eventHead + `,"result":{"schemaVersion":"1.0","requestId":"r1","executionId":"x1","status":"RESULT_STATUS_SUCCESS","startedAt":"2026-09-19T10:00:02Z","endedAt":"2026-09-19T10:00:03Z",` +
		`"toolProtocolStatus":"ok","resultSchemaValid":true,"resultHash":"sha256:44","redactedResultPreview":"{}","retryable":true,"sideEffectConfirmation":"confirmed",` +
		`"executedActionDigest":"sha256:22","redactionProfile":"default"}}`,
	"finding": `{` + eventHead + `,"finding":{"findingId":"f1","ruleId":"rule","ruleVersion":"1","severity":"FINDING_SEVERITY_HIGH","verdict":"FINDING_VERDICT_CONFIRMED","requestId":"r1","runId":"run1",` +
		`"evidenceRefs":["e1"],"frameworkMappings":["m"],"recommendedAction":"act","source":"FINDING_SOURCE_DETERMINISTIC","confidencePermille":900}}`,
	"policy": `{` + eventHead + `,"policy":{"bundleId":"b1","version":"2","digest":"sha256:33","createdAt":"2026-09-19T09:00:00Z"}}`,
}

// Decoding strictly proves the mirror names every field; writing the decoded
// event back and comparing proves none of them landed nowhere.
func TestMirrorCarriesEveryFieldOfTheContract(t *testing.T) {
	for payload, line := range everyFieldLines {
		t.Run(payload, func(t *testing.T) {
			events, err := evidence.DecodeJSONL(strings.NewReader(line+"\n"), 1)
			if err != nil {
				t.Fatalf("DecodeJSONL: %v", err)
			}
			written, err := json.Marshal(events[0])
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var want, got map[string]any
			if err := json.Unmarshal([]byte(line), &want); err != nil {
				t.Fatalf("literal: %v", err)
			}
			if err := json.Unmarshal(written, &got); err != nil {
				t.Fatalf("written: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("round trip lost or moved a field\n got: %s\nwant: %s", written, line)
			}
		})
	}
}

func TestMirrorReadsTheFieldsAddedAtThePin(t *testing.T) {
	events := decode(t, everyFieldLines["decision"]+"\n"+everyFieldLines["result"]+"\n")
	if got := events[0].PrevEventDigest; got != "sha256:00" {
		t.Errorf("prevEventDigest = %q, want sha256:00", got)
	}
	if got := events[0].Decision.DecidedAt.UTC().Format("15:04:05"); got != "10:00:01" {
		t.Errorf("decidedAt = %q, want 10:00:01", got)
	}
	if got := events[1].Result.RedactionProfile; got != "default" {
		t.Errorf("redactionProfile = %q, want default", got)
	}
}
