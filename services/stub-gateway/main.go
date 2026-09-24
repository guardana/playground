// The stub gateway decides nothing.
//
// It stands in for the enforcement plane, which is not built yet, so the runner
// can be proved to assert on decisions before there is anything that makes one.
// Every verdict it returns was written down by a scenario author in the file at
// LAB_STUB_VERDICTS; this service reads that file, replays it step by step, and
// records what it did. There is no rule in here, no policy, and nothing that
// looks at the arguments of a call.
//
// What it does do is real: it proxies the victim tool servers, carrying their
// annotations through unchanged so a tool that lies about being read-only still
// lies on the far side of the gateway, and it writes the evidence trail in the
// shape internal/evidence reads. A step nobody declared a verdict for is
// answered VERDICT_INDETERMINATE with STUB_NO_DECLARED_VERDICT and is not forwarded,
// because a trajectory longer than its verdict file has run past the end of
// what anybody stated.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// shutdownGrace bounds how long a stopping gateway waits for calls in flight.
// The trail is flushed after it, so the bound is also how long the runner waits
// for the file it reads.
const shutdownGrace = 5 * time.Second

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger); err != nil {
		logger.Error("stub-gateway stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) (err error) {
	settings, err := readConfig(os.LookupEnv)
	if err != nil {
		return err
	}
	declared, err := loadVerdicts(settings.verdicts)
	if err != nil {
		return err
	}
	file, err := openTrail(settings.trailPath())
	if err != nil {
		return err
	}
	// The runner reads this file after the container has stopped, so a trail
	// left unflushed would make a recorded decision look like one nobody made.
	defer func() { err = errors.Join(err, file.Sync(), file.Close()) }()

	upstreams, err := dialUpstreams(ctx, settings, upstreamTimeout)
	if err != nil {
		return err
	}
	defer closeAll(upstreams)

	records := newTrail(file, runIdentity{
		runID:     settings.runID,
		projectID: declared.ProjectID,
		tenantID:  declared.TenantID,
	})
	server, err := newGateway(settings.serverName, declared, records).newServer(ctx, upstreams)
	if err != nil {
		return err
	}
	logger.Info("stub-gateway ready",
		"listen", settings.listen, "run", settings.runID,
		"upstreams", len(upstreams), "declared_steps", len(declared.Steps))
	return serve(ctx, settings.listen, newMux(server))
}

// newMux serves the MCP endpoint and the readiness endpoint on one listener,
// so what compose waits on is the process that answers the calls.
func newMux(server *mcp.Server) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		// Reached only after every upstream answered tools/list, so a gateway
		// that is up is a gateway with the tools registered.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

func serve(ctx context.Context, address string, handler http.Handler) error {
	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	stopped := make(chan error, 1)
	go func() { stopped <- server.ListenAndServe() }()

	select {
	case err := <-stopped:
		return err
	case <-ctx.Done():
		grace, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
		defer cancel()
		return server.Shutdown(grace)
	}
}

// openTrail opens the evidence file for appending. The run directory is created
// if the runner has not already made it, so a lab brought up by hand records
// its calls rather than failing to start.
func openTrail(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- the path is the run directory the runner configured.
}

// dialUpstreams connects to every victim named in the environment.
//
// It fails on the first upstream it cannot reach rather than starting without
// it: a gateway missing a victim's tools would answer the agent's call with
// "no such tool", which is not the denial any scenario declared.
func dialUpstreams(ctx context.Context, settings config, timeout time.Duration) ([]upstream, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: settings.serverName, Version: stubVersion}, nil)
	connected := make([]upstream, 0, len(settings.upstreams))
	for _, ref := range settings.upstreams {
		session, err := dial(ctx, client, ref, timeout)
		if err != nil {
			closeAll(connected)
			return nil, fmt.Errorf("stub-gateway: connecting to %s at %s: %w", ref.name, ref.endpoint, err)
		}
		connected = append(connected, upstream{name: ref.name, session: session})
	}
	return connected, nil
}

// dial connects to one victim, bounded twice over.
//
// The deadline is the bound; the HTTP client's timeout is what makes it one. The
// SDK issues the handshake's requests on a context of its own, so a victim that
// accepts the connection and then says nothing would hold the POST open however
// long this context allows. The timeout on the client bounds every request the
// session ever makes, which is why it is the same value: a call is bounded by
// its own deadline first, and by this only if that deadline could not be
// delivered.
//
// The session outlives the context the handshake ran under. The SDK's client
// connection does not propagate that cancellation, so what is bounded here is
// reaching the victim, not talking to it afterwards.
func dial(ctx context.Context, client *mcp.Client, ref upstreamRef, timeout time.Duration) (*mcp.ClientSession, error) {
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return client.Connect(bounded, &mcp.StreamableClientTransport{
		Endpoint:   ref.endpoint,
		HTTPClient: &http.Client{Timeout: timeout},
		// Without this the client holds a standalone SSE stream open for the
		// life of the session, and the server it is connected to blocks on
		// close.
		DisableStandaloneSSE: true,
	}, nil)
}

func closeAll(upstreams []upstream) {
	for _, up := range upstreams {
		_ = up.session.Close()
	}
}
