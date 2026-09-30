package evidence_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/evidence"
)

// The unit tests use a namespace of their own: the reader takes it as a
// parameter, and only the golden carries the enforcer's.
const namespace = "lab.test"

type attr struct{ key, value string }

func a(name, value string) attr { return attr{namespace + "." + name, value} }

// body is one event line as the enforcer writes it, less the run id: compact,
// project p, tenant t, mode ENFORCE.
func body(id, request, kind, prev string) string {
	line := `{"eventId":"` + id + `","kind":"` + kind + `","requestId":"` + request +
		`","projectId":"p","tenantId":"t","enforcementMode":"ENFORCEMENT_MODE_ENFORCE"`
	if prev != "" {
		line += `,"prevEventId":"` + prev + `"`
	}
	return line + "}"
}

// attrs is what the enforcer writes beside body(id, request, kind, ...).
func attrs(id, request, kind string) []attr {
	return []attr{
		a("event_id", id), a("request_id", request), a("project_id", "p"), a("tenant_id", "t"),
		a("kind", kind), a("enforcement_mode", "ENFORCEMENT_MODE_ENFORCE"),
	}
}

func with(list []attr, name, value string) []attr {
	out := without(list, name)
	return append(out, a(name, value))
}

func without(list []attr, name string) []attr {
	var out []attr
	for _, item := range list {
		if item.key != namespace+"."+name {
			out = append(out, item)
		}
	}
	return out
}

// logRecord spells a record the way the collector's file exporter does:
// severity as a number, an observed time, flags.
func logRecord(bodyLine string, list []attr) string {
	quoted, _ := json.Marshal(bodyLine)
	return record(`{"stringValue":`+string(quoted)+`}`, list)
}

func record(rawBody string, list []attr) string {
	parts := make([]string, 0, len(list))
	for _, item := range list {
		key, _ := json.Marshal(item.key)
		value, _ := json.Marshal(item.value)
		parts = append(parts, `{"key":`+string(key)+`,"value":{"stringValue":`+string(value)+`}}`)
	}
	out := `{"timeUnixNano":"1789812000000000000","observedTimeUnixNano":"1789812000000000001","severityNumber":9,"severityText":"INFO"`
	if rawBody != "" {
		out += `,"body":` + rawBody
	}
	return out + `,"attributes":[` + strings.Join(parts, ",") + `],"flags":0}`
}

// export is one ExportLogsServiceRequest on one line, as the file exporter
// writes each batch it receives.
func export(records ...string) string {
	return `{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"svc"}}]},` +
		`"scopeLogs":[{"scope":{"name":"` + namespace + `","version":"1"},"logRecords":[` +
		strings.Join(records, ",") + `]}],"schemaUrl":"https://example.invalid/schema"}]}` + "\n"
}

func proposed(id, request string) string {
	return logRecord(body(id, request, "EVENT_KIND_ACTION_PROPOSED", ""),
		attrs(id, request, "EVENT_KIND_ACTION_PROPOSED"))
}

func decided(id, request, prev string) string {
	return logRecord(body(id, request, "EVENT_KIND_POLICY_DECIDED", prev),
		attrs(id, request, "EVENT_KIND_POLICY_DECIDED"))
}

func decodeOTLP(t *testing.T, input string, limit int) []evidence.Event {
	t.Helper()
	events, err := evidence.DecodeOTLP(strings.NewReader(input), namespace, limit)
	if err != nil {
		t.Fatalf("DecodeOTLP: %v", err)
	}
	return events
}

func eventIDs(events []evidence.Event) string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.EventID)
	}
	return strings.Join(ids, ",")
}
