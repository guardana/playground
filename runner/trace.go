package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/internal/runfile"
	"github.com/guardana/playground/runner/check"
)

const (
	// traceFile is what the agent writes and the verifier reads, in the run's
	// agent/ and verifier/ directories.
	traceFile     = "trace.jsonl"
	traceService  = "trace-verifier"
	maxTraceBytes = 8 << 20
)

// analyzeTrace hands the agent's trace to the verifier with the scenario's
// contract and returns what the verifier's report is graded on. The trace is
// copied into the verifier's own directory, the only one it mounts.
func (l lab) analyzeTrace(ctx context.Context, compose Compose, spec labspec.Scenario, runID, runDir string) check.Trace {
	dir := filepath.Join(runDir, "verifier")
	analysis := check.VerifierRun{
		URL:    path.Join(verifierMount, traceFile) + "#" + runID,
		Report: filepath.Join(dir, "trace-report.json"),
	}
	graded := check.Trace{Analysis: analysis, Want: *spec.Expect.Trace}
	if err := copyTrace(filepath.Join(runDir, "agent", traceFile), dir); err != nil {
		graded.Analysis.Detail = "the agent's trace could not be handed over: " + err.Error()
		return graded
	}
	split, err := compose.RunSplit(ctx, spec.Profile, traceService, "", []string{
		"analyze-trace", path.Join(verifierMount, traceFile),
		"--contract", path.Join("/contracts", filepath.Base(spec.Trace.Contract)),
		"--ai-system", spec.Trace.AISystem, "--format", "json",
	})
	if err != nil {
		graded.Analysis.Detail = err.Error()
		return graded
	}
	if err := keepNew(analysis.Report, split.Stdout, 0o600); err != nil {
		graded.Analysis.Detail = "the report could not be kept: " + err.Error()
		return graded
	}
	graded.Analysis.Ran, graded.Analysis.ExitCode = true, split.ExitCode
	graded.Analysis.Detail = fmt.Sprintf("exit %d", split.ExitCode)
	return graded
}

// copyTrace hands the agent's trace to the verifier in a directory it makes
// for it. The agent wrote the file from inside a container, so only a regular
// file within the bound is taken: a link there would hand the verifier a file
// of the host's, and an endless one the runner's memory.
func copyTrace(from, directory string) error {
	body, err := runfile.ReadRegular(from, maxTraceBytes)
	if err != nil {
		return err
	}
	if err := os.Mkdir(directory, 0o755); err != nil { // #nosec G301,G703 -- inside the run directory; the verifier's uid reads it.
		return err
	}
	if err := os.Chmod(directory, 0o755); err != nil { // #nosec G302,G703 -- the umask narrowed it; the verifier's uid reads it.
		return err
	}
	return keepNew(filepath.Join(directory, traceFile), string(body), 0o644)
}
