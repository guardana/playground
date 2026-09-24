package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/journal"
)

const (
	testIdentifier = "https://pdp.lab.test/pdp"
	testEvaluation = "/pdp/access/v1/evaluation"
	testRunID      = "run-pdp-1"
)

// harness is one double served by httptest over the double's own TLS, and a
// client configured as control's is: the lab CA as its only root, HTTP/2
// attempted, redirects not followed.
type harness struct {
	base        string
	evaluation  string
	client      *http.Client
	journalPath string
	writer      *journal.Writer
}

func startDouble(t *testing.T, scriptYAML string, hold time.Duration) *harness {
	t.Helper()
	sc, err := parseScript([]byte(scriptYAML))
	if err != nil {
		t.Fatalf("parseScript: %v", err)
	}
	base, err := checkIdentifier(testIdentifier)
	if err != nil {
		t.Fatalf("checkIdentifier: %v", err)
	}
	s := settings{identifier: testIdentifier, base: base, hold: hold, runID: testRunID, reportsDir: t.TempDir()}
	if err := os.MkdirAll(filepath.Dir(s.journalPath()), 0o750); err != nil {
		t.Fatal(err)
	}
	writer, err := journal.Open(s.journalPath(), serverName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	d, err := newDouble(s, sc, writer, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ca, err := newLabCA([]string{"127.0.0.1"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := ca.issueServer([]string{"127.0.0.1"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(d)
	server.EnableHTTP2 = true
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{leaf}}
	server.StartTLS()
	t.Cleanup(server.Close)
	return &harness{base: server.URL, evaluation: testEvaluation, client: controlLikeClient(t, ca.pem), journalPath: s.journalPath(), writer: writer}
}

func controlLikeClient(t *testing.T, caPEM []byte) *http.Client {
	t.Helper()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("the CA PEM holds no certificate")
	}
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool},
			ForceAttemptHTTP2: true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// questionFor is an evaluation request in the shape of control's mapping
// version 1, with synthetic identities.
func questionFor(action, resourceType, resourceID, subjectType, subjectID, requestID string) string {
	return fmt.Sprintf(`{"subject":{"type":%q,"id":%q,"properties":{"tenant_id":"tenant-a","authn_strength":"mfa","attributes":{}}},`+
		`"action":{"name":%q,"properties":{"kind":"tool_call","provider":"victim-crm","protocol":"mcp","effect":"MUTATE"}},`+
		`"resource":{"type":%q,"id":%q,"properties":{"tenant_id":"tenant-a","environment":"lab","labels":{}}},`+
		`"context":{"agent":{"id":"agent-1","framework":"scripted"},"destination":{"trust_zone":"INTERNAL","host":"victim-crm"},`+
		`"data":{"sensitivities":[],"contains_secrets":false},"project_id":"proj-a","tenant_id":"tenant-a",`+
		`"environment":"lab","request_id":%q,"supported_obligations":[]}}`,
		subjectType, subjectID, action, resourceType, resourceID, requestID)
}

// post sends one question as control's client does and returns the raw
// exchange under the context it ran in.
func (h *harness) post(t *testing.T, timeout time.Duration, requestID, body string) (context.Context, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.base+h.evaluation, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", requestID)
	resp, err := h.client.Do(req)
	return ctx, resp, err
}

// ask returns the reason code control would record for one question.
func (h *harness) ask(t *testing.T, timeout time.Duration, requestID, body string) string {
	t.Helper()
	ctx, resp, err := h.post(t, timeout, requestID, body)
	return controlReads(ctx, resp, err, requestID)
}

// askReading returns the reason code control would record for one question
// and the answer body it read to decide.
func (h *harness) askReading(t *testing.T, timeout time.Duration, requestID, body string) (string, []byte) {
	t.Helper()
	ctx, resp, err := h.post(t, timeout, requestID, body)
	if err != nil {
		return controlReads(ctx, nil, err, requestID), nil
	}
	text, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("reading the answer: %v", err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(text))
	return controlReads(ctx, resp, nil, requestID), text
}

func (h *harness) entries(t *testing.T) []journal.Entry {
	t.Helper()
	entries, err := journal.ReadFile(h.journalPath)
	if err != nil {
		t.Fatalf("reading the journal: %v", err)
	}
	return entries
}

// scriptAnswering scripts one answer for crm.refund and nothing else.
func scriptAnswering(answer string) string {
	return "schema_version: 1\nrules:\n  - match: {action: crm.refund}\n    answer: " + answer + "\n"
}
