package mcpserve_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/victims/mcpserve"
)

func TestConfigFromEnv(t *testing.T) {
	full := map[string]string{
		"LAB_LISTEN":      ":8080",
		"LAB_SERVER_NAME": "victim-fs",
		"LAB_RUN_ID":      "run-1",
		"LAB_REPORTS_DIR": "/reports",
	}

	tests := []struct {
		name    string
		missing string
	}{
		{name: "complete"},
		{name: "no listen", missing: "LAB_LISTEN"},
		{name: "no server name", missing: "LAB_SERVER_NAME"},
		{name: "no run id", missing: "LAB_RUN_ID"},
		{name: "no reports dir", missing: "LAB_REPORTS_DIR"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lookup := func(name string) string {
				if name == test.missing {
					return ""
				}
				return full[name]
			}

			config, err := mcpserve.ConfigFromEnv(lookup)
			if test.missing != "" {
				if err == nil {
					t.Fatalf("config accepted a missing %s", test.missing)
				}
				if !strings.Contains(err.Error(), test.missing) {
					t.Errorf("error %q does not name %s", err, test.missing)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := filepath.Join("/reports", "run-1", "journals", "victim-fs.jsonl")
			if got := config.JournalPath(); got != want {
				t.Errorf("journal path is %q, want %q", got, want)
			}
		})
	}
}

// A tool that answers without journalling is a hole in every effect assertion,
// so the wrapper is what records, and both paths go through it.
func TestJournalledRecordsEveryPath(t *testing.T) {
	type input struct {
		Fail bool `json:"fail"`
	}
	type output struct {
		Text string `json:"text"`
	}

	tests := []struct {
		name       string
		fail       bool
		wantStatus journal.Status
	}{
		{name: "an answered call is served", fail: false, wantStatus: journal.Served},
		{name: "a refused call is refused", fail: true, wantStatus: journal.Refused},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := configIn(t.TempDir())
			recorder, err := mcpserve.OpenJournal(config)
			if err != nil {
				t.Fatal(err)
			}

			handler := mcpserve.Journalled(recorder, "probe.run",
				func(_ context.Context, in input) (output, string, error) {
					if in.Fail {
						return output{}, "asked to fail", errors.New("refused")
					}
					return output{Text: "answered"}, "answered", nil
				})

			// Built the way a victim is, so the test also pins that one call
			// leaves exactly one line: the refusal middleware must not file a
			// second one for a call a handler already recorded.
			server := mcpserve.NewServer("victim-probe", recorder)
			mcp.AddTool(server, &mcp.Tool{Name: "probe.run"}, handler)

			session := connect(t, server)
			result, err := session.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      "probe.run",
				Arguments: input{Fail: test.fail},
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError != test.fail {
				t.Errorf("IsError is %v, want %v", result.IsError, test.fail)
			}
			if err := recorder.Close(); err != nil {
				t.Fatal(err)
			}

			entry := onlyEntry(t, config.JournalPath())
			if entry.Status != test.wantStatus {
				t.Errorf("status is %q, want %q", entry.Status, test.wantStatus)
			}
			if entry.Server != "victim-probe" {
				t.Errorf("server is %q, want victim-probe", entry.Server)
			}
			if entry.RunID != "run-1" {
				t.Errorf("run id is %q, want run-1", entry.RunID)
			}
		})
	}
}

// Compose waits on this endpoint, so it has to answer only once the listener is
// up: a health check that passes early starts a scenario against a server that
// is not there.
func TestHealthzAnswersOnlyWhenReady(t *testing.T) {
	tests := []struct {
		name  string
		ready bool
		want  int
	}{
		{name: "not ready", ready: false, want: http.StatusServiceUnavailable},
		{name: "ready", ready: true, want: http.StatusOK},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "probe", Version: "0"}, nil)
			handler := mcpserve.NewHandler(server, func() bool { return test.ready })

			request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			recorded := httptest.NewRecorder()
			handler.ServeHTTP(recorded, request)

			if recorded.Code != test.want {
				t.Errorf("/healthz answered %d, want %d", recorded.Code, test.want)
			}
		})
	}
}

func TestHandlerServesMCPOverHTTP(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "probe", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "probe.run"},
		func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			return nil, nil, nil
		})

	httpServer := httptest.NewServer(mcpserve.NewHandler(server, func() bool { return true }))
	defer httpServer.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	// Without this the standalone SSE stream stays open and Close blocks for
	// five seconds waiting for it.
	transport := &mcp.StreamableClientTransport{
		Endpoint:             httpServer.URL + "/mcp",
		DisableStandaloneSSE: true,
	}
	session, err := client.Connect(t.Context(), transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "probe.run" {
		t.Errorf("listed %d tools, want probe.run", len(tools.Tools))
	}
}

// journal.ReadFile refuses a line past journal.MaxLineBytes and returns nothing
// at all, so one oversized argument takes the ordinary calls recorded after it
// down with it. The reader is not the victim's to fix; what a victim writes is.
// A served call missing from the journal reads as a call that never arrived,
// and that is the direction that hides a defect.
func TestAHugeArgumentLeavesTheJournalReadable(t *testing.T) {
	type input struct {
		Statement string `json:"statement"`
	}

	config := configIn(t.TempDir())
	recorder, err := mcpserve.OpenJournal(config)
	if err != nil {
		t.Fatal(err)
	}

	server := mcpserve.NewServer("victim-probe", recorder)
	mcp.AddTool(server, &mcp.Tool{Name: "probe.run"},
		mcpserve.Journalled(recorder, "probe.run",
			func(_ context.Context, in input) (input, string, error) {
				return in, in.Statement, nil
			}))

	session := connect(t, server)
	huge := strings.Repeat("x", 2*journal.MaxLineBytes)
	for _, statement := range []string{huge, "select 1"} {
		if _, err := session.CallTool(t.Context(), &mcp.CallToolParams{
			Name:      "probe.run",
			Arguments: input{Statement: statement},
		}); err != nil {
			t.Fatal(err)
		}
	}
	// The refusal middleware files the tool name the client sent, so a name
	// nothing serves is the same hole by another route.
	if _, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      huge,
		Arguments: input{Statement: "select 1"},
	}); err == nil {
		t.Fatal("a tool the server does not have was accepted")
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}

	entries, err := journal.ReadFile(config.JournalPath())
	if err != nil {
		t.Fatalf("the journal cannot be read: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("journal has %d entries, want 3", len(entries))
	}
	if entries[1].Detail != "select 1" {
		t.Errorf("the ordinary call after the huge one reads %q", entries[1].Detail)
	}
	if !strings.Contains(entries[0].Detail, "truncated") {
		t.Errorf("a shortened detail does not say it was shortened: %q", shorten(entries[0].Detail))
	}
	if !strings.Contains(entries[2].Tool, "truncated") {
		t.Errorf("a shortened tool name does not say it was shortened: %q", shorten(entries[2].Tool))
	}
	assertLinesFitTheReader(t, config.JournalPath())
}

// A line has to stay clear of the reader's limit rather than merely under it:
// the limit covers the timestamp, the server, the tool, the run id and the
// status as well as the detail, and a cap set close to it is one field away
// from taking the whole file down again.
func assertLinesFitTheReader(t *testing.T, path string) {
	t.Helper()

	const headroom = 10

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for number, line := range strings.Split(strings.TrimSuffix(string(body), "\n"), "\n") {
		if len(line) >= journal.MaxLineBytes/headroom {
			t.Errorf("line %d is %d bytes; the reader stops at %d", number+1, len(line), journal.MaxLineBytes)
		}
	}
}

func shorten(text string) string {
	const enough = 120
	if len(text) <= enough {
		return text
	}
	return text[:enough] + "..."
}

// onlyEntry reads the journal and insists there is exactly one line in it: a
// call that leaves two is counted twice, and a call that leaves none is
// counted as a call that never arrived.
func onlyEntry(t *testing.T, path string) journal.Entry {
	t.Helper()

	entries, err := journal.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("journal has %d entries, want 1", len(entries))
	}
	return entries[0]
}

func configIn(directory string) mcpserve.Config {
	return mcpserve.Config{
		Listen:     ":0",
		ServerName: "victim-probe",
		RunID:      "run-1",
		ReportsDir: directory,
	}
}

func connect(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()

	session, err := mcpserve.ConnectInMemory(t.Context(), server)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// A call the SDK rejects never reaches a tool, so nothing inside the handler
// can record it. Left unrecorded, a call the victim refused would be
// indistinguishable from a call the gateway stopped before it arrived, and
// those are the two answers a denial scenario has to tell apart.
func TestACallRefusedBeforeTheToolRunsIsJournalled(t *testing.T) {
	type input struct {
		Path string `json:"path"`
	}

	tests := []struct {
		name      string
		tool      string
		arguments any
		wantTool  string
	}{
		{name: "unknown tool", tool: "probe.missing", arguments: input{Path: "/x"}, wantTool: "probe.missing"},
		{name: "arguments the schema rejects", tool: "probe.run", arguments: map[string]any{"path": 7}, wantTool: "probe.run"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := configIn(t.TempDir())
			recorder, err := mcpserve.OpenJournal(config)
			if err != nil {
				t.Fatal(err)
			}

			server := mcpserve.NewServer("victim-probe", recorder)
			mcp.AddTool(server, &mcp.Tool{Name: "probe.run"},
				mcpserve.Journalled(recorder, "probe.run",
					func(_ context.Context, in input) (input, string, error) {
						return in, in.Path, nil
					}))

			session := connect(t, server)
			// An unknown tool comes back as a protocol error and bad arguments
			// as an error result; neither reaches a handler.
			result, err := session.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      test.tool,
				Arguments: test.arguments,
			})
			if err == nil && !result.IsError {
				t.Fatal("the call was accepted")
			}
			if err := recorder.Close(); err != nil {
				t.Fatal(err)
			}

			entry := onlyEntry(t, config.JournalPath())
			if entry.Status != journal.Refused {
				t.Errorf("status is %q, want %q", entry.Status, journal.Refused)
			}
			if entry.Tool != test.wantTool {
				t.Errorf("tool is %q, want %q", entry.Tool, test.wantTool)
			}
		})
	}
}
