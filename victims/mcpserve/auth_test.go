package mcpserve_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/victims/mcpserve"
)

// gatedVictim serves one journalled tool behind go-sdk's bearer-token check,
// with JournalAuth outermost, the way a token-gated victim is built.
func gatedVictim(t *testing.T, token string) (string, *mcpserve.Recorder, mcpserve.Config) {
	t.Helper()
	config := configIn(t.TempDir())
	recorder, err := mcpserve.OpenJournal(config)
	if err != nil {
		t.Fatal(err)
	}
	server := mcpserve.NewServer("victim-probe", recorder)
	mcp.AddTool(server, &mcp.Tool{Name: "probe.run"},
		mcpserve.Journalled(recorder, "probe.run", func(context.Context, struct{}) (struct{}, string, error) {
			return struct{}{}, "ran", nil
		}))
	verify := func(_ context.Context, presented string, _ *http.Request) (*auth.TokenInfo, error) {
		if presented != token {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{Expiration: time.Now().Add(time.Hour)}, nil
	}
	handler, err := mcpserve.NewRoutedHandler(server, func() bool { return true }, mcpserve.Routing{
		Wrap: func(next http.Handler) http.Handler {
			return recorder.JournalAuth(token, auth.RequireBearerToken(verify, nil)(next))
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	return httpServer.URL + "/mcp", recorder, config
}

// rpc posts one JSON-RPC message with the given Authorization and session,
// either left out when empty, and returns the status and any session issued.
func rpc(t *testing.T, url, authorization, session, body string) (int, string) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
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
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	return response.StatusCode, response.Header.Get("Mcp-Session-Id")
}

const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe","version":"0"}}}`

// Each request leaves one auth line saying what it carried and what it was
// answered, and the one call that reached the tool is the journal's only call.
func TestJournalAuthRecordsEachDecisionAndNoCall(t *testing.T) {
	token := strings.Repeat("vK", 21) + "w"
	url, recorder, config := gatedVictim(t, token)
	session := everyCase(t, url, token)
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	assertAuthLines(t, config.JournalPath())

	calls, err := journal.ReadFile(config.JournalPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].Tool != "probe.run" || calls[0].Status != journal.Served {
		t.Errorf("calls = %+v, want probe.run served once", calls)
	}
	body, err := os.ReadFile(config.JournalPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), token) || strings.Contains(string(body), session) {
		t.Error("the journal holds the token or the session id")
	}
}

// everyCase sends one request per case the auth lines tell apart and returns
// the session the server issued.
func everyCase(t *testing.T, url, token string) string {
	t.Helper()
	bearer := "Bearer " + token
	const list = `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`
	rpc(t, url, "", "", initialize)
	_, session := rpc(t, url, bearer, "", initialize)
	if session == "" {
		t.Fatal("initialize with the run's token issued no session")
	}
	rpc(t, url, bearer, session, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	rpc(t, url, bearer, session, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"probe.run","arguments":{}}}`)
	rpc(t, url, "", session, list)
	rpc(t, url, "Bearer "+strings.Repeat("x", 43), session, list)
	rpc(t, url, bearer, "session-never-issued", list)
	rpc(t, url, bearer, session, `{"jsonrpc":"2.0","id":4,"method":"`+token+`"}`)
	return session
}

func assertAuthLines(t *testing.T, path string) {
	t.Helper()
	lines, err := journal.ReadAuthFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"none none 401 initialize",
		"run none 200 initialize",
		"run known 202 notifications/initialized",
		"run known 200 tools/call",
		"none known 401 tools/list",
		"other known 401 tools/list",
		"run unknown 404 tools/list",
		"run known 400 ",
	}
	got := make([]string, 0, len(lines))
	for _, line := range lines {
		got = append(got, fmt.Sprintf("%s %s %d %s", line.Credential, line.Session, line.Status, line.Method))
		if line.Server != "victim-probe" || line.RunID != "run-1" || line.OccurredAt.IsZero() {
			t.Errorf("line not stamped: %+v", line)
		}
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("auth lines:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A decision the journal could not take is not answered as made.
func TestJournalAuthAnswersNothingItCouldNotRecord(t *testing.T) {
	token := strings.Repeat("vK", 21) + "w"
	url, recorder, _ := gatedVictim(t, token)
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	status, session := rpc(t, url, "Bearer "+token, "", initialize)
	if status != http.StatusInternalServerError || session != "" {
		t.Errorf("an unrecorded initialize answered %d with session %q, want 500 and none", status, session)
	}
}
