package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testRunID = "run-auth-1"

// served is one double running in-process on a loopback port, as main runs it.
type served struct {
	base        string
	personas    string
	journalPath string
	done        chan error
	cancel      context.CancelFunc
}

// startDouble serves a double over personas, a map from persona name to the
// bytes of its file, with LAB_AUTH_PERSONA set to persona unless it is empty.
func startDouble(t *testing.T, personas map[string]string, persona string) *served {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "personas")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for name, body := range personas {
		if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	env := map[string]string{"LAB_RUN_ID": testRunID, "LAB_REPORTS_DIR": filepath.Join(root, "reports")}
	if persona != "" {
		env[personaVariable] = persona
	}
	s, err := parseSettings([]string{"-personas", dir}, lookupFrom(env))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveOn(ctx, s, listener, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	srv := &served{
		base: "http://" + listener.Addr().String(), personas: dir, done: done, cancel: cancel,
		journalPath: filepath.Join(root, "reports", testRunID, "journals", "auth-double.jsonl"),
	}
	t.Cleanup(func() { srv.stop(t) })
	return srv
}

func lookupFrom(env map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := env[name]
		return value, ok
	}
}

// stop ends the double and fails the test when it had stopped on its own or
// does not stop cleanly.
func (s *served) stop(t *testing.T) {
	t.Helper()
	select {
	case err := <-s.done:
		t.Fatalf("the double stopped on its own: %v", err)
	default:
	}
	s.cancel()
	select {
	case err := <-s.done:
		if err != nil {
			t.Errorf("serveOn returned %v on shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("serveOn did not return after its context ended")
	}
}

// get sends one request with headers and returns the status and the body.
func (s *served) get(t *testing.T, method, path string, headers map[string]string) (int, []byte, http.Header) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, s.base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, body, resp.Header
}

// lines reads the double's journal as written, and its raw bytes.
func (s *served) lines(t *testing.T) ([]line, []byte) {
	t.Helper()
	return readLines(t, s.journalPath)
}

func readLines(t *testing.T, path string) ([]line, []byte) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the journal: %v", err)
	}
	var parsed []line
	for _, text := range bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n")) {
		if len(text) == 0 {
			continue
		}
		var entry line
		if err := json.Unmarshal(text, &entry); err != nil {
			t.Fatalf("journal line %q: %v", text, err)
		}
		parsed = append(parsed, entry)
	}
	return parsed, raw
}
