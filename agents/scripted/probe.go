package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"
)

// The runner reads these two markers out of the probe's output, and repeats
// them in runner/compose.go because the two are separate commands. An exit code
// on its own cannot tell "the agent image would not run" from "the agent ran
// and found no route", and only the second is evidence about the topology.
const (
	probeReached     = "probe reached"
	probeUnreachable = "probe unreachable"
)

// probeTimeout bounds one dial. It is short because a probe is asking whether a
// route exists, and a route that takes seconds to answer is a finding of its
// own rather than something to wait out.
const probeTimeout = 3 * time.Second

// probe reports whether a TCP connection to address could be established.
//
// Every way of failing is one answer. A victim on tool-net does not resolve
// from agent-net at all, because Docker's embedded DNS answers only for the
// networks a container is attached to, so an unknown host is as much "no route"
// as a refused connection is.
func probe(ctx context.Context, address string) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	var dialer net.Dialer
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	return connection.Close()
}

// runProbe writes the one line the runner reads, and reports the failure so the
// process exits non-zero.
func runProbe(ctx context.Context, address string, out io.Writer) error {
	if err := probe(ctx, address); err != nil {
		if _, writeErr := fmt.Fprintf(out, "%s %s: %v\n", probeUnreachable, address, err); writeErr != nil {
			return writeErr
		}
		return fmt.Errorf("%s: %w", address, err)
	}
	_, err := fmt.Fprintf(out, "%s %s\n", probeReached, address)
	return err
}
