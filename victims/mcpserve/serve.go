package mcpserve

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	readHeaderTimeout = 5 * time.Second
	// A call in flight is worth waiting for, because it is about to be
	// journalled. An event stream that will not drain is not.
	shutdownGrace = 5 * time.Second
)

// NewHandler serves MCP at /mcp and readiness at /healthz on one listener.
//
// ready is asked on every health check rather than read once, because compose
// waits on this endpoint: a check that passes before the listener is up starts
// a scenario against a server that is not there, and the scenario reads the
// absence as a denial.
func NewHandler(server *mcp.Server, ready func() bool) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server }, nil))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		if !ready() {
			http.Error(w, "starting", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
	})
	return mux
}

// Run is the whole life of a victim: read the environment, open the journal,
// build the server from it, serve until asked to stop, close the journal.
//
// build takes the recorder because a tool handler cannot be written without
// one; it is the only thing a victim needs from this package before it has
// answered anything.
func Run(ctx context.Context, build func(*Recorder) (*mcp.Server, error)) error {
	config, err := ConfigFromEnv(os.Getenv)
	if err != nil {
		return err
	}
	recorder, err := OpenJournal(config)
	if err != nil {
		return err
	}
	server, err := build(recorder)
	if err != nil {
		return errors.Join(err, recorder.Close())
	}
	// The journal closes after the listener has stopped, so a call that was in
	// flight is in the file the runner reads.
	return errors.Join(Serve(ctx, config, server), recorder.Close())
}

// Serve listens on config.Listen until ctx is done or the process is asked to
// stop. SIGTERM is what compose sends, and it is the signal that has to reach
// the journal.
func Serve(ctx context.Context, config Config, server *mcp.Server) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	listener, err := net.Listen("tcp", config.Listen)
	if err != nil {
		return err
	}
	return serveOn(ctx, listener, server)
}

// serveOn serves on an already-open listener. The listener is the caller's, so
// its address is knowable before anything has been served on it.
func serveOn(ctx context.Context, listener net.Listener, server *mcp.Server) error {
	var ready atomic.Bool
	httpServer := &http.Server{
		Handler:           NewHandler(server, ready.Load),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	ready.Store(true)

	failed := make(chan error, 1)
	go func() { failed <- httpServer.Serve(listener) }()

	select {
	case err := <-failed:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		return shutdown(httpServer)
	}
}

func shutdown(httpServer *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		return errors.Join(err, httpServer.Close())
	}
	return nil
}

// ConnectInMemory connects a client to server over a transport pair inside this
// process. The per-server tests drive their tools through it, so what they
// exercise is the registered tool, its schema and its annotations, rather than
// a handler function called directly.
func ConnectInMemory(ctx context.Context, server *mcp.Server) (*mcp.ClientSession, error) {
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		return nil, err
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "lab-client", Version: "1"}, nil)
	return client.Connect(ctx, clientTransport, nil)
}
