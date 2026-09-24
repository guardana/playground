// The pdp-double stands in for an organization's AuthZEN decision point.
//
// It answers control's evaluation requests as a scenario's script says:
// allow, deny, allow with an obligation, hold past the caller's deadline,
// fail, omit the X-Request-ID echo, answer malformed, or add a member. A
// question no rule matches is denied. Every question it receives is journalled
// before it is answered, so a scenario can assert the plane asked, or never did.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/guardana/playground/internal/journal"
)

const shutdownGrace = 5 * time.Second

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.LookupEnv, logger); err != nil {
		logger.Error("pdp-double stopped", "error", err)
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

// serveOn serves https on listener until ctx ends. The CA file is written
// only once everything that can refuse to start has been checked, so its
// presence means the double is about to answer.
func serveOn(ctx context.Context, s settings, listener net.Listener, logger *slog.Logger) (err error) {
	defer func() { _ = listener.Close() }()
	sc, err := loadScript(s.script)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.journalPath()), 0o750); err != nil {
		return err
	}
	writer, err := journal.Open(s.journalPath(), serverName)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, writer.Close()) }()
	d, err := newDouble(s, sc, writer, logger)
	if err != nil {
		return err
	}
	serving, err := startTLS(s)
	if err != nil {
		return err
	}
	logger.Info("pdp-double ready", "listen", listener.Addr().String(), "identifier", s.identifier,
		"rules", len(sc.Rules), "run", s.runID)
	return serve(ctx, d, tls.NewListener(listener, serving), s.hold)
}

// startTLS makes the CA and the serving certificate and hands the gateway the
// CA certificate. No key is written anywhere.
func startTLS(s settings) (*tls.Config, error) {
	now := time.Now()
	ca, err := newLabCA(s.sans, now)
	if err != nil {
		return nil, fmt.Errorf("pdp-double: making the CA: %w", err)
	}
	leaf, err := ca.issueServer(s.sans, now)
	if err != nil {
		return nil, fmt.Errorf("pdp-double: issuing the serving certificate: %w", err)
	}
	if err := writeCA(s.caOut, ca.pem); err != nil {
		return nil, fmt.Errorf("pdp-double: writing the CA certificate: %w", err)
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{leaf},
		NextProtos:   []string{"h2", "http/1.1"},
	}, nil
}

func serve(ctx context.Context, handler http.Handler, listener net.Listener, hold time.Duration) error {
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		// A held answer has to outlive the hold, or the bound is the server's.
		WriteTimeout: hold + 10*time.Second,
		IdleTimeout:  time.Minute,
		// Held requests end with the process rather than holding up shutdown.
		BaseContext: func(net.Listener) context.Context { return ctx },
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
