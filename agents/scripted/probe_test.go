package main

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestProbeReportsWhetherATCPConnectionCouldBeMade(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

	if err := probe(context.Background(), listener.Addr().String()); err != nil {
		t.Errorf("a listening port was reported unreachable: %v", err)
	}

	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := closed.Addr().String()
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := probe(context.Background(), address); err == nil {
		t.Error("a closed port was reported reachable")
	}
}

// A victim on tool-net does not resolve from agent-net at all: Docker's
// embedded DNS answers only for the networks a container is attached to. A name
// that does not resolve is "not reachable" like any other, and the reason it
// gave is what tells a person which of the three it was.
func TestProbeTreatsANameThatDoesNotResolveAsUnreachable(t *testing.T) {
	err := probe(context.Background(), "victim-fs.this-name-does-not-resolve.invalid:8080")
	if err == nil {
		t.Fatal("an unresolvable name was reported reachable")
	}
	if !strings.Contains(err.Error(), "victim-fs.this-name-does-not-resolve.invalid") {
		t.Errorf("the failure does not name what was dialled: %v", err)
	}
}

func TestProbeWritesALineTheRunnerCanTellApartFromAFailureToRunAtAll(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

	var out strings.Builder
	if err := runProbe(context.Background(), listener.Addr().String(), &out); err != nil {
		t.Fatalf("runProbe: %v", err)
	}
	if !strings.HasPrefix(out.String(), probeReached+" ") {
		t.Errorf("a reached probe wrote %q", out.String())
	}

	out.Reset()
	if err := runProbe(context.Background(), "127.0.0.1:1", &out); err == nil {
		t.Error("a refused connection exited zero")
	}
	if !strings.HasPrefix(out.String(), probeUnreachable+" ") {
		t.Errorf("an unreachable probe wrote %q", out.String())
	}
	if !strings.Contains(out.String(), "127.0.0.1:1") {
		t.Errorf("the line does not name what was dialled: %q", out.String())
	}
}
