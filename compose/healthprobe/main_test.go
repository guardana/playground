package main

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShowPrintsTheStatusAndTheBodyWhateverTheStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"halted":true}`))
	}))
	defer server.Close()
	var out strings.Builder
	if err := show(http.DefaultClient, server.URL, &out); err != nil {
		t.Fatalf("show: %v", err)
	}
	if out.String() != "status 503\n{\"halted\":true}" {
		t.Errorf("show printed %q", out.String())
	}
	if err := probe(http.DefaultClient, server.URL); err == nil {
		t.Error("probe passed a 503")
	}
}

func TestShowRefusesAnOversizedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxShown+1)))
	}))
	defer server.Close()
	var out strings.Builder
	if err := show(http.DefaultClient, server.URL, &out); err == nil {
		t.Error("show printed a body past its bound")
	}
}

func TestTheCAFlagTrustsExactlyThatCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	if err := probe(http.DefaultClient, server.URL); err == nil {
		t.Fatal("a server under an unknown CA was trusted without -ca")
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	block := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(path, block, 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := trust(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := probe(client, server.URL); err != nil {
		t.Errorf("the server's own CA was not trusted: %v", err)
	}
	if _, err := trust(filepath.Join(t.TempDir(), "absent.pem")); err == nil {
		t.Error("a missing CA file was accepted")
	}
}
