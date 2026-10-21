package mcpserve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
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
	return fixedRoutes(streamable(server), ready)
}

// Routing is what a victim serves beside its tools. The zero value serves /mcp
// bare and /healthz, and nothing else.
type Routing struct {
	// Wrap wraps the MCP handler: a bearer-token check, and JournalAuth around
	// it. Nil serves the MCP handler as it is.
	Wrap func(http.Handler) http.Handler
	// Routes are further patterns this server answers, such as its protected
	// resource metadata. A pattern on /mcp or /healthz is refused: those two
	// answer the same way on every victim.
	Routes map[string]http.Handler
}

// NewRoutedHandler is NewHandler with a victim's own routing. A route it
// cannot register is an error, never a panic.
func NewRoutedHandler(server *mcp.Server, ready func() bool, routing Routing) (http.Handler, error) {
	handler := streamable(server)
	if routing.Wrap != nil {
		if handler = routing.Wrap(handler); handler == nil {
			return nil, errors.New("mcpserve: the wrapper returned no MCP handler")
		}
	}
	mux := fixedRoutes(handler, ready)
	for _, pattern := range slices.Sorted(maps.Keys(routing.Routes)) {
		if err := addRoute(mux, pattern, routing.Routes[pattern]); err != nil {
			return nil, err
		}
	}
	return mux, nil
}

func streamable(server *mcp.Server) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
}

// fixedRoutes serves /mcp and /healthz, the two routes every victim answers.
func fixedRoutes(mcpHandler http.Handler, ready func() bool) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
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

// addRoute registers one of a victim's routes. A pattern naming a method or a
// host is more specific than /mcp and would take it over without a conflict,
// so the fixed paths are refused by path before the mux sees them.
func addRoute(mux *http.ServeMux, pattern string, handler http.Handler) (err error) {
	slash := strings.Index(pattern, "/")
	switch {
	case handler == nil:
		return fmt.Errorf("mcpserve: route %q has no handler", pattern)
	case slash < 0:
		return fmt.Errorf("mcpserve: route %q names no path", pattern)
	case pattern[slash:] == "/mcp" || pattern[slash:] == "/healthz":
		return fmt.Errorf("mcpserve: route %q would change what every victim serves at %s", pattern, pattern[slash:])
	}
	// The mux reports a malformed or conflicting pattern only by panicking.
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("mcpserve: route %q: %v", pattern, recovered)
		}
	}()
	mux.Handle(pattern, handler)
	return nil
}

// Run is the whole life of a victim: read the environment, open the journal,
// build the server from it, serve until asked to stop, close the journal.
//
// build takes the recorder because a tool handler cannot be written without
// one; it is the only thing a victim needs from this package before it has
// answered anything.
func Run(ctx context.Context, build func(*Recorder) (*mcp.Server, error)) error {
	return RunRouted(ctx, func(recorder *Recorder) (*mcp.Server, Routing, error) {
		server, err := build(recorder)
		return server, Routing{}, err
	})
}

// RunRouted is Run for a victim that serves more than its tools: build also
// returns its Routing, which it can only write once the recorder is open.
func RunRouted(ctx context.Context, build func(*Recorder) (*mcp.Server, Routing, error)) error {
	config, err := ConfigFromEnv(os.Getenv)
	if err != nil {
		return err
	}
	recorder, err := OpenJournal(config)
	if err != nil {
		return err
	}
	server, routing, err := build(recorder)
	if err != nil {
		return errors.Join(err, recorder.Close())
	}
	// The journal closes after the listener has stopped, so a call that was in
	// flight is in the file the runner reads.
	return errors.Join(serve(ctx, config, server, routing), recorder.Close())
}

// Serve listens on config.Listen until ctx is done or the process is asked to
// stop. SIGTERM is what compose sends, and it is the signal that has to reach
// the journal.
func Serve(ctx context.Context, config Config, server *mcp.Server) error {
	return serve(ctx, config, server, Routing{})
}

func serve(ctx context.Context, config Config, server *mcp.Server, routing Routing) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	listener, err := net.Listen("tcp", config.Listen)
	if err != nil {
		return err
	}
	return serveOn(ctx, listener, server, routing)
}

// serveOn serves on an already-open listener. The listener is the caller's, so
// its address is knowable before anything has been served on it.
func serveOn(ctx context.Context, listener net.Listener, server *mcp.Server, routing Routing) error {
	var ready atomic.Bool
	handler, err := NewRoutedHandler(server, ready.Load, routing)
	if err != nil {
		return errors.Join(err, listener.Close())
	}
	httpServer := &http.Server{
		Handler:           handler,
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
