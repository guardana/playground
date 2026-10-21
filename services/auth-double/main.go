// The auth-double stands in for the authorization server a protected MCP
// server names in its resource metadata.
//
// It serves one RFC 8414 metadata document, the persona a scenario chose, as
// written, so a persona can omit or change any field. With no persona, or one
// that does not load, it stays up and answers 404 to every metadata request.
// Every request is journalled with whether it carried an Authorization or an
// Mcp-Session-Id header, never their values.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const shutdownGrace = 5 * time.Second

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.LookupEnv, logger); err != nil {
		logger.Error("auth-double stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, lookup func(string) (string, bool), logger *slog.Logger) error {
	s, err := parseSettings(args, lookup)
	if err != nil {
		return err
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.listen)
	if err != nil {
		return err
	}
	return serveOn(ctx, s, listener, logger)
}

// serveOn serves http on listener until ctx ends. Only the journal can stop
// it from starting; a persona that does not load leaves it answering 404.
func serveOn(ctx context.Context, s settings, listener net.Listener, logger *slog.Logger) (err error) {
	defer func() { _ = listener.Close() }()
	if err := os.MkdirAll(filepath.Dir(s.journalPath()), 0o750); err != nil {
		return err
	}
	j, err := openJournal(s.journalPath())
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, j.close()) }()
	d := &double{journal: j, runID: s.runID, logger: logger}
	if d.metadata, err = loadPersona(s); err != nil {
		logger.Warn("auth-double answers 404 to every metadata request", "error", err)
	} else {
		d.persona = s.persona
	}
	logger.Info("auth-double ready", "listen", listener.Addr().String(), "persona", d.persona, "run", s.runID)
	return serve(ctx, d, listener)
}

func serve(ctx context.Context, handler http.Handler, listener net.Listener) error {
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       time.Minute,
		MaxHeaderBytes:    64 << 10,
	}
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()

	select {
	case err := <-stopped:
		return err
	case <-ctx.Done():
		grace, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
		defer cancel()
		return server.Shutdown(grace)
	}
}
