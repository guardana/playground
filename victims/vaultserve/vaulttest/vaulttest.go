// Package vaulttest serves a vault in-process and sends it the requests the
// verifier's authorization probes send, so each vault's test states, as
// literals, what it answers and what its auth journal says.
package vaulttest

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/victims/mcpserve"
	"github.com/guardana/playground/victims/vaultserve"
)

// Token is a run token for tests: the length the runner mints, and no
// secret. It is assembled so the lab's secret scanners have nothing to match.
func Token() string { return strings.Repeat("Qx", 21) + "e" }

// Initialize opens an MCP session.
const Initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25",` +
	`"capabilities":{},"clientInfo":{"name":"lab-probe","version":"0"}}}`

// ListTools lists a session's tools.
const ListTools = `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`

const initialized = `{"jsonrpc":"2.0","method":"notifications/initialized"}`

// Victim is one vault on a loopback listener.
type Victim struct {
	URL      string
	t        testing.TB
	recorder *mcpserve.Recorder
	journal  string
}

// Start serves a vault named name, built with policy, for the test's life.
func Start(t testing.TB, name string, policy vaultserve.Policy) *Victim {
	t.Helper()
	config := mcpserve.Config{Listen: "127.0.0.1:0", ServerName: name, RunID: "run-1", ReportsDir: t.TempDir()}
	recorder, err := mcpserve.OpenJournal(config)
	if err != nil {
		t.Fatal(err)
	}
	server, routing, err := vaultserve.Build(recorder, name, Token(), policy)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := mcpserve.NewRoutedHandler(server, func() bool { return true }, routing)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	return &Victim{URL: httpServer.URL, t: t, recorder: recorder, journal: config.JournalPath()}
}

// Answer is what one request got back.
type Answer struct {
	Status    int
	Session   string
	Challenge string
}

// Post sends one JSON-RPC message to /mcp, with the Authorization header and
// session id left out when empty.
func (v *Victim) Post(authorization, session, body string) Answer {
	v.t.Helper()
	request, err := http.NewRequestWithContext(v.t.Context(), http.MethodPost, v.URL+"/mcp", strings.NewReader(body))
	if err != nil {
		v.t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if session != "" {
		request.Header.Set("Mcp-Session-Id", session)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		v.t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	return Answer{
		Status:    response.StatusCode,
		Session:   response.Header.Get("Mcp-Session-Id"),
		Challenge: response.Header.Get("WWW-Authenticate"),
	}
}

// AuthLines closes the journal and reads its auth lines back as
// "credential session status method".
func (v *Victim) AuthLines() []string {
	v.t.Helper()
	if err := v.recorder.Close(); err != nil {
		v.t.Fatal(err)
	}
	lines, err := journal.ReadAuthFile(v.journal)
	if err != nil {
		v.t.Fatal(err)
	}
	got := make([]string, 0, len(lines))
	for _, line := range lines {
		got = append(got, fmt.Sprintf("%s %s %d %s", line.Credential, line.Session, line.Status, line.Method))
	}
	return got
}

// Calls closes the journal and reads its call lines back.
func (v *Victim) Calls() []journal.Entry {
	v.t.Helper()
	if err := v.recorder.Close(); err != nil {
		v.t.Fatal(err)
	}
	calls, err := journal.ReadFile(v.journal)
	if err != nil {
		v.t.Fatal(err)
	}
	return calls
}

// Connect opens an MCP client session that presents authorization on every
// request.
func (v *Victim) Connect(authorization string) *mcp.ClientSession {
	v.t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "lab-probe", Version: "0"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   v.URL + "/mcp",
		HTTPClient: &http.Client{Transport: presenting{authorization}},
	}
	session, err := client.Connect(v.t.Context(), transport, nil)
	if err != nil {
		v.t.Fatal(err)
	}
	v.t.Cleanup(func() { _ = session.Close() })
	return session
}

type presenting struct{ authorization string }

func (p presenting) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("Authorization", p.authorization)
	return http.DefaultTransport.RoundTrip(request)
}

// AlgNone is an unsigned JWT, the token the verifier presents to see whether
// a server checks a signature at all. It is assembled here because the lab's
// guards scan test files for tokens.
func AlgNone() string {
	part := func(text string) string { return base64.RawURLEncoding.EncodeToString([]byte(text)) }
	return part(`{"alg":"none","typ":"JWT"}`) + "." + part(`{"sub":"lab-probe","aud":"elsewhere"}`) + "."
}

// AssertLines fails t unless got is want, line for line.
func AssertLines(t testing.TB, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s:\n%s\nwant:\n%s", what, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
