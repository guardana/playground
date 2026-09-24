package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func (h *harness) get(t *testing.T, path string) (*http.Response, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, body
}

// The metadata check is control's doctor: one strict object whose two members
// equal the configured identifier and the endpoint derived from it.
func TestMetadataNamesTheIdentifierAndEndpointControlDerives(t *testing.T) {
	h := startDouble(t, scriptAnswering("allow"), 10*time.Second)
	resp, body := h.get(t, "/.well-known/authzen-configuration/pdp")
	if resp.StatusCode != http.StatusOK || !declaresJSON(resp.Header.Values("Content-Type")) {
		t.Fatalf("status %d, type %q, want a 200 of JSON", resp.StatusCode, resp.Header.Values("Content-Type"))
	}
	doc, err := strictObject(body)
	if err != nil {
		t.Fatalf("metadata is not one strict object: %v", err)
	}
	var id, endpoint string
	if json.Unmarshal(doc["policy_decision_point"], &id) != nil || json.Unmarshal(doc["access_evaluation_endpoint"], &endpoint) != nil {
		t.Fatalf("metadata members are not strings: %s", body)
	}
	if id != "https://pdp.lab.test/pdp" || endpoint != "https://pdp.lab.test/pdp/access/v1/evaluation" {
		t.Fatalf("metadata names %q and %q", id, endpoint)
	}
	if entries := h.entries(t); len(entries) != 0 {
		t.Fatalf("a metadata fetch was journalled as a question: %+v", entries)
	}
}

func TestHealthzAnswersAndOtherPathsDoNot(t *testing.T) {
	h := startDouble(t, scriptAnswering("allow"), 10*time.Second)
	if resp, _ := h.get(t, "/healthz"); resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status %d", resp.StatusCode)
	}
	if resp, _ := h.get(t, "/access/v1/evaluation"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("an evaluation outside the identifier's path answered %d, want 404", resp.StatusCode)
	}
}
