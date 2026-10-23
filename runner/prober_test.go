package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/guardana/playground/drivers/guardana"
)

// The verifier's files, services and bounds as the compose topology and its
// driver name them, written out so a fixture cannot borrow them from the
// driver it tests.
const (
	verifierDir        = "verifier"
	verifierPinProfile = "verifier-pin"
	scanService        = "scan-verifier"
	traceService       = "trace-verifier"
	inputsDir          = "in"
	scanTarget         = "target"
	maxInputBytes      = 1 << 20
	maxScanFiles       = 256
	maxScanDepth       = 8
)

// writtenPin is where a pin step's pin lands on the host, under the run's
// verifier directory.
func writtenPin(dir string, step int) string {
	return filepath.Join(dir, fmt.Sprintf("step-%d", step), "out", "pin.json")
}

// reconfigureVerifier builds the lab's verifier again from its configuration
// with change applied.
func reconfigureVerifier(t testing.TB, subject *lab, change func(*guardana.Config)) {
	t.Helper()
	config := subject.verifier.Config()
	change(&config)
	subject.verifier = guardana.New(config)
}
