package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/evidence"
)

func TestATrailLinksItsEventsAndValidates(t *testing.T) {
	var file bytes.Buffer
	writer := newTrail(&file, runIdentity{runID: "run-1", projectID: "playground", tenantID: "tenant_a"})

	request := writer.request()
	for _, kind := range []evidence.Kind{
		evidence.KindActionProposed,
		evidence.KindPolicyDecided,
		evidence.KindActionBlocked,
	} {
		if err := request.append(evidence.Event{Kind: kind}); err != nil {
			t.Fatalf("append %s: %v", kind, err)
		}
	}

	events, err := evidence.DecodeJSONL(bytes.NewReader(file.Bytes()), 16)
	if err != nil {
		t.Fatalf("DecodeJSONL: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("decoded %d events, want 3", len(events))
	}
	if err := evidence.ValidateChain(events); err != nil {
		t.Fatalf("ValidateChain: %v", err)
	}
	for number, event := range events {
		if event.SchemaVersion != wireSchemaVersion {
			t.Errorf("event %d carries schemaVersion %q", number, event.SchemaVersion)
		}
		if event.RunID != "run-1" || event.ProjectID != "playground" || event.TenantID != "tenant_a" {
			t.Errorf("event %d carries run %q project %q tenant %q", number, event.RunID, event.ProjectID, event.TenantID)
		}
		if event.OccurredAt.IsZero() {
			t.Errorf("event %d carries no time", number)
		}
	}
}

// The trail is one file and calls arrive concurrently, so two requests are
// interleaved in it. Each has to be a coherent account on its own, which is
// what evidence.ByRequest and ValidateChain read together.
func TestTwoRequestsInOneFileEachValidate(t *testing.T) {
	var file bytes.Buffer
	writer := newTrail(&file, runIdentity{runID: "run-1", projectID: "playground", tenantID: "tenant_a"})

	first, second := writer.request(), writer.request()
	order := []struct {
		request *requestTrail
		kind    evidence.Kind
	}{
		{first, evidence.KindActionProposed},
		{second, evidence.KindActionProposed},
		{second, evidence.KindPolicyDecided},
		{first, evidence.KindPolicyDecided},
		{first, evidence.KindActionBlocked},
		{second, evidence.KindActionBlocked},
	}
	for _, step := range order {
		if err := step.request.append(evidence.Event{Kind: step.kind}); err != nil {
			t.Fatalf("append %s: %v", step.kind, err)
		}
	}

	events, err := evidence.DecodeJSONL(bytes.NewReader(file.Bytes()), 16)
	if err != nil {
		t.Fatalf("DecodeJSONL: %v", err)
	}
	trails := evidence.ByRequest(events)
	if len(trails) != 2 {
		t.Fatalf("%d trails, want 2", len(trails))
	}
	for id, trail := range trails {
		if err := evidence.ValidateChain(trail); err != nil {
			t.Errorf("ValidateChain %s: %v", id, err)
		}
	}
}

// One event is one line. A payload holding a newline that reached the file
// unescaped would split one event into two unreadable ones.
func TestOneEventIsOneLine(t *testing.T) {
	var file bytes.Buffer
	writer := newTrail(&file, runIdentity{runID: "run-1", projectID: "p", tenantID: "t"})
	err := writer.request().append(evidence.Event{
		Kind:     evidence.KindActionProposed,
		Proposed: &evidence.ActionEnvelope{Action: &evidence.Action{Name: "two\nlines"}},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if lines := strings.Count(file.String(), "\n"); lines != 1 {
		t.Errorf("%d newlines in one event", lines)
	}
	var decoded map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(file.Bytes()), &decoded); err != nil {
		t.Fatalf("the line is not one JSON object: %v", err)
	}
}
