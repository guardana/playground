package main

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/journal"
)

type served struct {
	root, caPath, addr string
	done               chan error
	cancel             context.CancelFunc
}

func serveInProcess(t *testing.T, scriptPath string) *served {
	t.Helper()
	root := t.TempDir()
	caPath := filepath.Join(root, "pki", "pdp-ca.pem")
	env := map[string]string{"LAB_RUN_ID": "run-e2e", "LAB_REPORTS_DIR": filepath.Join(root, "reports")}
	s, err := parseSettings([]string{
		"-identifier", "https://localhost", "-san", "localhost,127.0.0.1",
		"-ca-out", caPath, "-script", scriptPath,
	}, lookupFrom(env))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- serveOn(ctx, s, listener, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	return &served{root: root, caPath: caPath, addr: listener.Addr().String(), done: done, cancel: cancel}
}

func TestServedOverHTTPSWithTheWrittenCAAndNoKeyOnDisk(t *testing.T) {
	scriptPath := filepath.Join(t.TempDir(), "pdp.yaml")
	if err := os.WriteFile(scriptPath, []byte(scriptAnswering("allow")), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := serveInProcess(t, scriptPath)
	caPEM := waitForFile(t, srv.caPath)
	h := &harness{base: "https://" + srv.addr, evaluation: "/access/v1/evaluation", client: controlLikeClient(t, caPEM)}
	ctx, resp, err := h.post(t, 5*time.Second, "req-e2e", questionFor("crm.refund", "order", "ord-1", "user", "user-7", "req-e2e"))
	if got := controlReads(ctx, resp, err, "req-e2e"); got != "PDP_ALLOW" {
		t.Fatalf("control reads %s, want PDP_ALLOW", got)
	}
	if resp.ProtoMajor != 2 {
		t.Errorf("negotiated %s, want HTTP/2 as control's transport attempts", resp.Proto)
	}
	srv.cancel()
	select {
	case err := <-srv.done:
		if err != nil {
			t.Fatalf("serveOn returned %v on shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveOn did not return after its context ended")
	}
	assertNoKeyUnder(t, srv.root)
	entries, err := journal.ReadFile(filepath.Join(srv.root, "reports", "run-e2e", "journals", "pdp-double.jsonl"))
	if err != nil || len(entries) != 1 || entries[0].Tool != "crm.refund" || entries[0].Detail != "allow" || entries[0].RunID != "run-e2e" {
		t.Fatalf("journal at the run's path: %+v, %v", entries, err)
	}
}

func TestNoCAIsWrittenWhenTheDoubleCannotStart(t *testing.T) {
	srv := serveInProcess(t, filepath.Join(t.TempDir(), "missing.yaml"))
	select {
	case err := <-srv.done:
		if err == nil {
			t.Fatal("serveOn started without a script")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveOn did not refuse a missing script")
	}
	if _, err := os.Stat(srv.caPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the CA certificate was written by a double that did not start: %v", err)
	}
}

func waitForFile(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if body, err := os.ReadFile(path); err == nil {
			return body
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s was not written within 5s", path)
	return nil
}

func assertNoKeyUnder(t *testing.T, dir string) {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		body, err := fs.ReadFile(root.FS(), path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), "PRIVATE"+" KEY") {
			t.Errorf("%s holds a private key", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
