package dockercompose

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ContainerImage is the image ID the project's own container of service runs.
func (d CLI) ContainerImage(ctx context.Context, profiles []string, service string) (string, error) {
	listed, err := d.capture(ctx, profiles, "ps", "--all", "-q", service)
	if err != nil {
		return "", err
	}
	ids := strings.Fields(listed)
	if len(ids) != 1 {
		return "", fmt.Errorf("the project has %d %s containers, want one", len(ids), service)
	}
	return Output(ctx, "docker", "inspect", "--format", "{{.Image}}", ids[0])
}

// Output runs one host program and returns its trimmed output, or the first
// line of what it said on failure.
func Output(ctx context.Context, name string, args ...string) (string, error) {
	// #nosec G204,G702 -- fixed programs; the arguments are the lab's own pins and paths.
	run := exec.CommandContext(ctx, name, args...)
	var stderr strings.Builder
	run.Stderr = &stderr
	out, err := run.Output()
	if err != nil {
		if said, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n"); said != "" {
			return "", fmt.Errorf("%w: %s", err, said)
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
