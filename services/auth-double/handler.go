package main

import (
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"
)

// metadataPath is RFC 8414's well-known location for an issuer with no path,
// which is what every persona names.
const metadataPath = "/.well-known/oauth-authorization-server"

// double answers the metadata request with its persona, or 404 without one,
// and journals every request before answering it.
type double struct {
	journal *journalFile
	runID   string
	// metadata is the persona file as written; nil when none loaded.
	metadata []byte
	persona  string
	logger   *slog.Logger
}

func (d *double) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	health := r.Method == http.MethodGet && r.URL.Path == "/healthz"
	if health && fromLoopback(r.RemoteAddr) {
		_, _ = io.WriteString(w, "ok\n")
		return
	}
	status := d.statusFor(r, health)
	// A request the journal did not take is one a scenario could not see was
	// made, so it is never answered with the metadata.
	if !d.record(r, status) {
		http.Error(w, "journal unavailable", http.StatusServiceUnavailable)
		return
	}
	switch {
	case status == http.StatusNotFound:
		http.NotFound(w, r)
	case health:
		_, _ = io.WriteString(w, "ok\n")
	default:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(d.metadata)
	}
}

func (d *double) statusFor(r *http.Request, health bool) int {
	if health || (r.Method == http.MethodGet && r.URL.Path == metadataPath && d.metadata != nil) {
		return http.StatusOK
	}
	return http.StatusNotFound
}

func (d *double) record(r *http.Request, status int) bool {
	err := d.journal.record(line{
		OccurredAt:    time.Now().UTC(),
		Server:        serverName,
		RunID:         d.runID,
		Method:        r.Method,
		Path:          r.URL.EscapedPath(),
		Authorization: len(r.Header.Values("Authorization")) > 0,
		McpSessionID:  len(r.Header.Values("Mcp-Session-Id")) > 0,
		Query:         r.URL.RawQuery != "",
		Headers:       headerNames(r.Header),
		Status:        status,
		Persona:       d.persona,
	})
	if err != nil {
		d.logger.Error("journal write failed", "path", r.URL.EscapedPath(), "error", err)
		return false
	}
	return true
}

// fromLoopback reports whether the request came from inside the container,
// where the health probe runs.
func fromLoopback(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// headerNames lists the names a request carried, never their values.
func headerNames(header http.Header) string {
	return strings.Join(slices.Sorted(maps.Keys(header)), ",")
}
