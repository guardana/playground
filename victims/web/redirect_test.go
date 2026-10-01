package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/victims/mcpserve"
)

// The redirect target answers, so a client that follows it gets a page back:
// a refusal here is the host check, not a name that failed to resolve.
func TestARedirectToAnotherHostIsRefused(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("a page from another host"))
	}))
	t.Cleanup(elsewhere.Close)
	pages := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/landing.html", http.StatusFound)
	}))
	t.Cleanup(pages.Close)
	origin, err := url.Parse(pages.URL)
	if err != nil {
		t.Fatal(err)
	}
	config := mcpserve.Config{Listen: ":0", ServerName: "victim-web", RunID: "run-1", ReportsDir: t.TempDir()}
	recorder, err := mcpserve.OpenJournal(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close() })
	session, err := mcpserve.ConnectInMemory(t.Context(), newServerWith(recorder, origin.Host, pages.Client()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "web.fetch", Arguments: map[string]any{"url": pages.URL + "/moved.html"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := text(t, result); !result.IsError || strings.Contains(got, "another host") || !strings.Contains(got, "host is not allowed") {
		t.Errorf("a redirect to another host was followed or not refused for its host: %s", got)
	}
	entries, err := journal.ReadFile(config.JournalPath())
	if err != nil || len(entries) != 1 || entries[0].Status != journal.Refused {
		t.Errorf("journal = %v, %v, want one refused line", entries, err)
	}
}

// A page that redirects to itself would otherwise be followed until the fetch
// timed out; the client stops after the cap net/http itself keeps.
func TestARedirectLoopStopsAtTheCap(t *testing.T) {
	var pages *httptest.Server
	pages = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, pages.URL+"/again.html", http.StatusFound)
	}))
	t.Cleanup(pages.Close)
	origin, err := url.Parse(pages.URL)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (fetcher{client: pages.Client(), host: origin.Host}).fetch(t.Context(), pages.URL+"/start.html")
	if err == nil || !strings.Contains(err.Error(), "redirects") {
		t.Errorf("a redirect loop was answered %+v, %v; want a refusal naming the redirects", got, err)
	}
}
