package evidence_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/evidence"
)

// The collector's file exporter writes one request per line, and a request may
// hold several resources and scopes. Every record is read, in file order.
func TestDecodeOTLPReadsTheCollectorFile(t *testing.T) {
	second := strings.Replace(export(decided("a2", "ra", "a1")), `]}],"schemaUrl"`,
		`]},{"scope":{"name":"`+namespace+`"},"logRecords":[`+proposed("b1", "rb")+`]}],"schemaUrl"`, 1)
	events := decodeOTLP(t, export(proposed("a1", "ra"))+second, 8)
	if got := eventIDs(events); got != "a1,a2,b1" {
		t.Fatalf("events = %s, want a1,a2,b1", got)
	}
	if events[2].RequestID != "rb" || events[1].PrevEventID != "a1" {
		t.Errorf("b1 request = %q, a2 prev = %q", events[2].RequestID, events[1].PrevEventID)
	}
}

// Attributes outside the namespace belong to whoever added them; a key that
// merely starts with the namespace's letters is not inside it.
func TestDecodeOTLPIgnoresAttributesOutsideTheNamespace(t *testing.T) {
	list := append(attrs("a1", "ra", "EVENT_KIND_ACTION_PROPOSED"),
		attr{namespace + "ing.event_id", "zz"}, attr{namespace, "zz"}, attr{"other.kind", "zz"})
	events := decodeOTLP(t, export(logRecord(body("a1", "ra", "EVENT_KIND_ACTION_PROPOSED", ""), list)), 1)
	if got := eventIDs(events); got != "a1" {
		t.Fatalf("events = %s, want a1", got)
	}
}

// The enforcer writes an enum's zero by its name, while the body leaves it out.
func TestDecodeOTLPReadsAnUnspecifiedModeAsAgreeing(t *testing.T) {
	line := strings.Replace(body("a1", "ra", "EVENT_KIND_FINDING_RAISED", ""), `,"enforcementMode":"ENFORCEMENT_MODE_ENFORCE"`, "", 1)
	list := with(attrs("a1", "ra", "EVENT_KIND_FINDING_RAISED"), "enforcement_mode", "ENFORCEMENT_MODE_UNSPECIFIED")
	events := decodeOTLP(t, export(logRecord(line, list)), 1)
	if got := eventIDs(events); got != "a1" {
		t.Fatalf("events = %s, want a1", got)
	}
	if events[0].EnforcementMode != "" {
		t.Errorf("mode = %q, want empty", events[0].EnforcementMode)
	}
}

func TestDecodeOTLPReadsAnUnspecifiedKindAsAgreeing(t *testing.T) {
	line := strings.Replace(body("a1", "ra", "", ""), `"kind":"",`, "", 1)
	list := with(attrs("a1", "ra", ""), "kind", "EVENT_KIND_UNSPECIFIED")
	events := decodeOTLP(t, export(logRecord(line, list)), 1)
	if got := eventIDs(events); got != "a1" {
		t.Fatalf("events = %s, want a1", got)
	}
	if events[0].Kind != "" {
		t.Errorf("kind = %q, want empty", events[0].Kind)
	}
}

func TestDecodeOTLPRefusesAttributesThatDisagree(t *testing.T) {
	base := attrs("a1", "ra", "EVENT_KIND_ACTION_PROPOSED")
	cases := map[string][]attr{
		"event_id differs":          with(base, "event_id", "a2"),
		"request_id differs":        with(base, "request_id", "rb"),
		"project_id differs":        with(base, "project_id", "q"),
		"tenant_id differs":         with(base, "tenant_id", "u"),
		"kind differs":              with(base, "kind", "EVENT_KIND_POLICY_DECIDED"),
		"mode differs":              with(base, "enforcement_mode", "ENFORCEMENT_MODE_OBSERVE"),
		"kind unspecified":          with(base, "kind", "EVENT_KIND_UNSPECIFIED"),
		"mode unspecified":          with(base, "enforcement_mode", "ENFORCEMENT_MODE_UNSPECIFIED"),
		"event_id missing":          without(base, "event_id"),
		"request_id missing":        without(base, "request_id"),
		"project_id missing":        without(base, "project_id"),
		"tenant_id missing":         without(base, "tenant_id"),
		"kind missing":              without(base, "kind"),
		"mode missing":              without(base, "enforcement_mode"),
		"run_id the event lacks":    with(base, "run_id", "run9"),
		"run_id empty but present":  with(base, "run_id", ""),
		"unknown namespaced member": with(base, "surprise", "x"),
		"repeated key, same value":  append(base, a("event_id", "a1")),
	}
	line := body("a1", "ra", "EVENT_KIND_ACTION_PROPOSED", "")
	for name, list := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := evidence.DecodeOTLP(strings.NewReader(export(logRecord(line, list))), namespace, 1)
			if !errors.Is(err, evidence.ErrAttributeMismatch) {
				t.Fatalf("err = %v, want ErrAttributeMismatch", err)
			}
		})
	}
}

func TestDecodeOTLPRefusesABodyThatIsNotOneString(t *testing.T) {
	list := attrs("a1", "ra", "EVENT_KIND_ACTION_PROPOSED")
	cases := map[string]string{
		"missing":            "",
		"null":               "null",
		"empty":              "{}",
		"not a string":       `{"intValue":"3"}`,
		"number":             `{"stringValue":1}`,
		"null string":        `{"stringValue":null}`,
		"string and another": `{"stringValue":"{}","intValue":"3"}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := evidence.DecodeOTLP(strings.NewReader(export(record(raw, list))), namespace, 1)
			if !errors.Is(err, evidence.ErrMalformedOTLP) {
				t.Fatalf("err = %v, want ErrMalformedOTLP", err)
			}
		})
	}
}

func TestDecodeOTLPRefusesANamespacedAttributeThatIsNotAString(t *testing.T) {
	good := proposed("a1", "ra")
	bad := strings.Replace(good, `"value":{"stringValue":"a1"}`, `"value":{"intValue":"1"}`, 1)
	_, err := evidence.DecodeOTLP(strings.NewReader(export(bad)), namespace, 1)
	if !errors.Is(err, evidence.ErrMalformedOTLP) {
		t.Fatalf("err = %v, want ErrMalformedOTLP", err)
	}
}

func TestDecodeOTLPRefusesAnEnvelopeItCannotRead(t *testing.T) {
	cases := map[string]string{
		"null":              "null\n",
		"array":             "[]\n",
		"no resourceLogs":   "{}\n",
		"null resourceLogs": `{"resourceLogs":null}` + "\n",
		"cut short":         strings.TrimSuffix(export(proposed("a1", "ra")), "}]}\n"),
		"garbage after":     export(proposed("a1", "ra")) + "]\n",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := evidence.DecodeOTLP(strings.NewReader(input), namespace, 4)
			if !errors.Is(err, evidence.ErrMalformedOTLP) {
				t.Fatalf("err = %v, want ErrMalformedOTLP", err)
			}
		})
	}
}

// The body is held to the same line rules as the JSONL file.
func TestDecodeOTLPHoldsTheBodyToTheLineRules(t *testing.T) {
	list := attrs("a1", "ra", "EVENT_KIND_ACTION_PROPOSED")
	line := body("a1", "ra", "EVENT_KIND_ACTION_PROPOSED", "")
	cases := map[string]string{
		"unknown field": strings.Replace(line, `{"eventId"`, `{"surprise":1,"eventId"`, 1),
		"two lines":     line + "\n" + line,
		"newline after": line + "\n",
		"empty":         "",
		"no eventId":    strings.Replace(line, `"eventId":"a1",`, "", 1),
	}
	for name, bodyLine := range cases {
		t.Run(name, func(t *testing.T) {
			attrList := list
			if name == "no eventId" {
				attrList = without(list, "event_id")
			}
			_, err := evidence.DecodeOTLP(strings.NewReader(export(logRecord(bodyLine, attrList))), namespace, 1)
			if !errors.Is(err, evidence.ErrMalformedLine) {
				t.Fatalf("err = %v, want ErrMalformedLine", err)
			}
		})
	}
}
