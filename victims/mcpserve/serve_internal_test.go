package mcpserve

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/internal/journal"
)

// Compose waits on /healthz and then stops the container with SIGTERM. Both
// ends of that are here: the check answers on the listener the server is
// actually serving, and a stop returns rather than hanging, because the
// journal is closed after this returns.
func TestServeOnAnswersHealthAndStopsWithTheContext(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	stopped := make(chan error, 1)
	go func() {
		stopped <- serveOn(ctx, listener, mcp.NewServer(&mcp.Implementation{Name: "probe", Version: "0"}, nil))
	}()

	if code := health(t, listener.Addr().String()); code != http.StatusOK {
		t.Errorf("/healthz answered %d, want %d", code, http.StatusOK)
	}

	stop()
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("the server stopped with %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("the server did not stop when its context was cancelled")
	}
}

// health polls, because the listener is open before the server is marked
// ready. Retrying is the honest shape of the question compose asks.
func health(t *testing.T, address string) int {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	code := 0
	for time.Now().Before(deadline) {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
			"http://"+address+"/healthz", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		code = response.StatusCode
		_ = response.Body.Close()
		if code == http.StatusOK {
			return code
		}
		time.Sleep(10 * time.Millisecond)
	}
	return code
}

// Run is the wiring: environment, journal, server, listener, and the journal
// closed on the way out. The runner reads that file after the container has
// stopped, so a journal left unflushed makes a served call look like a call
// that never arrived.
func TestRunOpensTheJournalAndClosesIt(t *testing.T) {
	reports := t.TempDir()
	t.Setenv("LAB_LISTEN", "127.0.0.1:0")
	t.Setenv("LAB_SERVER_NAME", "victim-probe")
	t.Setenv("LAB_RUN_ID", "run-1")
	t.Setenv("LAB_REPORTS_DIR", reports)

	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	opened := make(chan *Recorder, 1)
	stopped := make(chan error, 1)
	go func() {
		stopped <- Run(ctx, func(recorder *Recorder) (*mcp.Server, error) {
			opened <- recorder
			return NewServer("victim-probe", recorder), nil
		})
	}()

	recorder := <-opened
	if err := recorder.Served("probe.run", "recorded before the stop"); err != nil {
		t.Fatal(err)
	}

	stop()
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}

	entries := readJournal(t, filepath.Join(reports, "run-1", "journals", "victim-probe.jsonl"))
	if len(entries) != 1 {
		t.Fatalf("journal has %d entries, want 1", len(entries))
	}
	if err := recorder.Served("probe.run", "after the stop"); err == nil {
		t.Error("the journal was still open after Run returned")
	}
}

func readJournal(t *testing.T, path string) []journal.Entry {
	t.Helper()

	entries, err := journal.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}
