// Package repofiles lists the files this repository owns by asking
// scripts/repo-files.sh, the one list every scan uses: tracked files and
// untracked ones git does not ignore, so run output under reports/ is never
// judged as documentation. Tracked narrows it to what a clone holds.
package repofiles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strings"
	"time"
)

// Script is the lister, relative to the repository root.
const Script = "scripts/repo-files.sh"

// Timeout bounds one listing; the script reads git's index and nothing else.
const Timeout = 30 * time.Second

// ErrInvalid is wrapped by every refusal of the lister's output.
var ErrInvalid = errors.New("repofiles")

// List runs the lister in root and returns its slash paths. An empty list is
// refused: a scan built on it would inspect nothing.
func List(ctx context.Context, root string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	command := exec.CommandContext(ctx, "./"+Script) // #nosec G204 -- a constant path.
	command.Dir = root
	var stderr bytes.Buffer
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", Script, err, strings.TrimSpace(stderr.String()))
	}
	return Parse(out)
}

// Parse reads the lister's output: one clean relative slash path per line.
func Parse(out []byte) ([]string, error) {
	text := strings.TrimSuffix(string(out), "\n")
	if text == "" {
		return nil, fmt.Errorf("%w: %s listed no file", ErrInvalid, Script)
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if err := checkLine(line); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
	}
	return lines, nil
}

func checkLine(line string) error {
	if line == "" || strings.HasPrefix(line, "/") || path.Clean(line) != line || strings.HasPrefix(line, "../") {
		return fmt.Errorf("%w: %q is not a clean relative path", ErrInvalid, line)
	}
	return nil
}
