package evidence_test

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/guardana/playground/internal/evidence"
)

// The request the enforcer's exporter is held to, decoded here, is the four
// events of the JSONL beside it. The namespace comes from the golden's own
// scope name, so no product name is spelled in code.
func TestGoldenRequestDecodesToTheGoldenEvents(t *testing.T) {
	request := readFile(t, "testdata/otlp/export_logs_request.json")
	want, err := evidence.DecodeJSONL(bytes.NewReader(readFile(t, "testdata/otlp/events.jsonl")), 4)
	if err != nil {
		t.Fatalf("DecodeJSONL: %v", err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, request); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	shapes := map[string][]byte{
		"control's request":  request,
		"one collector line": append(compact.Bytes(), '\n'),
	}
	for name, input := range shapes {
		t.Run(name, func(t *testing.T) {
			got, err := evidence.DecodeOTLP(bytes.NewReader(input), goldenNamespace(t, request), 4)
			if err != nil {
				t.Fatalf("DecodeOTLP: %v", err)
			}
			if ids := eventIDs(got); ids != "evt-1,evt-2,evt-3,evt-4" {
				t.Fatalf("events = %s, want evt-1,evt-2,evt-3,evt-4", ids)
			}
			if verdict := got[1].Decision.GetVerdict(); verdict != "VERDICT_DENY" {
				t.Errorf("evt-2 verdict = %q, want VERDICT_DENY", verdict)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("decoded request differs from events.jsonl")
			}
		})
	}
}

func goldenNamespace(t *testing.T, request []byte) string {
	t.Helper()
	var shape struct {
		ResourceLogs []struct {
			ScopeLogs []struct {
				Scope struct{ Name string } `json:"scope"`
			} `json:"scopeLogs"`
		} `json:"resourceLogs"`
	}
	if err := json.Unmarshal(request, &shape); err != nil || len(shape.ResourceLogs) != 1 ||
		len(shape.ResourceLogs[0].ScopeLogs) != 1 || shape.ResourceLogs[0].ScopeLogs[0].Scope.Name == "" {
		t.Fatalf("golden has no single scope name: %v", err)
	}
	return shape.ResourceLogs[0].ScopeLogs[0].Scope.Name
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}
