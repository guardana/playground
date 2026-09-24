// The approver stands in for the person who answers a held call.
//
// It lists the plane's approvals directory with the enforcer's own
// `approvals list` on an interval and, as a scenario's script says, approves,
// rejects or leaves each waiting approval to expire, optionally after a delay.
// Every answer goes through the enforcer's `approvals approve|reject`, which is
// the only writer of the directory. Everything it does, and every refusal of
// the command, is journalled, so a scenario grades the approval path from
// records.
package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/guardana/playground/internal/journal"
)

const shutdownGrace = 5 * time.Second

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := run(ctx, os.Args[1:], os.LookupEnv, os.Stderr, logger)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		logger.Error("approver stopped", "error", err)
		os.Exit(1)
	}
}

// run serves /healthz and answers approvals until ctx ends.
func run(ctx context.Context, args []string, lookup func(string) (string, bool), usage io.Writer, logger *slog.Logger) (err error) {
	s, err := parseSettings(args, lookup, usage)
	if err != nil {
		return err
	}
	sc, err := loadScript(s.script)
	if err != nil {
		return err
	}
	// #nosec G703 -- parseSettings refuses a run id with a separator or "..", so the path stays under the reports directory.
	if err := os.MkdirAll(filepath.Dir(s.journalPath()), 0o750); err != nil {
		return err
	}
	writer, err := journal.Open(s.journalPath(), serverName)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, writer.Close()) }()
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.listen)
	if err != nil {
		return err
	}
	var hl health
	server := &http.Server{Handler: healthMux(&hl), ReadHeaderTimeout: 5 * time.Second}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()

	a := newApprover(s.dir, commander{path: s.control, timeout: s.execTimeout}, sc, writer, s.runID, logger)
	logger.Info("approver ready", "dir", s.dir, "control", s.control, "rules", len(sc.Rules), "run", s.runID)
	poll(ctx, a, &hl, s.interval, logger)
	settleUnsure(a, logger)

	shutdown, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		return err
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// settleUnsure reads one last listing, with a context of its own because the
// run's is done, so an answer cut short at shutdown is journalled as what its
// record shows.
func settleUnsure(a *approver, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := a.settle(ctx); err != nil {
		logger.Error("answers cut short at shutdown were not settled", "error", err)
	}
}

// poll lists at once and then every interval until ctx ends. A failed listing,
// or one no plane holds, is logged and turns /healthz unhealthy; the next one
// may succeed.
func poll(ctx context.Context, a *approver, hl *health, interval time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		err := a.tick(ctx, time.Now())
		if err != nil && ctx.Err() == nil {
			logger.Error("approvals not dealt with", "error", err)
		}
		hl.set(err)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// health is healthy while the last listing went through and a plane held the
// directory, and never before the first such listing.
type health struct {
	mu   sync.Mutex
	ok   bool
	last string
}

func (h *health) set(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ok, h.last = err == nil, ""
	if err != nil {
		h.last = err.Error()
	}
}

func (h *health) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.mu.Lock()
	ok, last := h.ok, h.last
	h.mu.Unlock()
	if !ok {
		if last == "" {
			last = "no listing has succeeded yet"
		}
		http.Error(w, last, http.StatusServiceUnavailable)
		return
	}
	_, _ = io.WriteString(w, "ok\n")
}

func healthMux(h *health) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", h)
	return mux
}
