package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/guardana/playground/runner/report"
)

// provenanceTimeout bounds the git and docker lookups a report header needs.
const provenanceTimeout = 30 * time.Second

// Labels the image builds stamp with the pin each image was built from.
const (
	revisionLabel = "org.opencontainers.image.revision"
	versionLabel  = "org.opencontainers.image.version"
)

var pinLine = regexp.MustCompile(`^([A-Z][A-Z0-9_]*)=(.*)$`)

// lookup runs one program and returns its trimmed output; command is the real
// one, and a test hands in what the program would have said.
type lookup func(ctx context.Context, name string, args ...string) (string, error)

// readPins reads versions.env in file order. A line that is not a comment, not
// blank and not NAME=VALUE is refused rather than skipped.
func readPins(path string) ([]report.Pin, error) {
	file, err := os.Open(path) // #nosec G304 -- the lab's own versions.env.
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var pins []report.Pin
	scanner := bufio.NewScanner(file)
	for number := 1; scanner.Scan(); number++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		match := pinLine.FindStringSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("%s:%d: not NAME=VALUE: %q", path, number, line)
		}
		pins = append(pins, report.Pin{Name: match[1], Value: match[2]})
	}
	return pins, scanner.Err()
}

func pinValue(pins []report.Pin, name string) string {
	for _, pin := range pins {
		if pin.Name == name {
			return pin.Value
		}
	}
	return ""
}

// describeHost reads what produced a run from the lab's checkout, versions.env,
// the local Docker daemon and the runtime.
func describeHost(root string, run lookup) func(context.Context) report.Provenance {
	return func(ctx context.Context) report.Provenance {
		ctx, cancel := context.WithTimeout(ctx, provenanceTimeout)
		defer cancel()
		described := report.Provenance{Lab: labCommit(ctx, run, root), Machine: machine(ctx, run)}
		pins, err := readPins(filepath.Join(root, versionFile))
		if err != nil {
			described.Pins = []report.Pin{{Name: versionFile, Value: "unreadable: " + err.Error()}}
			return described
		}
		described.Pins = pins
		described.Images = images(ctx, run, pins)
		return described
	}
}

// images reads the enforcer by the commit its build stamps and the verifier by
// the release its build stamps.
func images(ctx context.Context, run lookup, pins []report.Pin) []report.Image {
	return []report.Image{
		inspectImage(ctx, run, pinValue(pins, "ENFORCER_IMAGE"), pinValue(pins, "ENFORCER_COMMIT"), revisionLabel),
		inspectImage(ctx, run, pinValue(pins, "VERIFIER_IMAGE"), pinValue(pins, "VERIFIER_VERSION"), versionLabel),
	}
}

func (l lab) provenance(ctx context.Context) report.Provenance {
	if l.describe == nil {
		return report.Provenance{}
	}
	return l.describe(ctx)
}

func labCommit(ctx context.Context, run lookup, root string) string {
	commit, err := run(ctx, "git", "-C", root, "rev-parse", "HEAD")
	if err != nil {
		return "not a git checkout: " + err.Error()
	}
	changed, err := run(ctx, "git", "-C", root, "status", "--porcelain", "--untracked-files=normal")
	switch {
	case err != nil:
		return commit + ", uncommitted changes unknown: " + err.Error()
	case changed != "":
		return commit + " with uncommitted changes"
	}
	return commit
}

func inspectImage(ctx context.Context, run lookup, name, pin, label string) report.Image {
	image := report.Image{Ref: name + ":" + pin, Want: pin}
	if name == "" || pin == "" {
		image.Missing = "versions.env names no image or no pin for it"
		return image
	}
	format := fmt.Sprintf(`{{.Id}} {{index .Config.Labels %q}}`, label)
	found, err := run(ctx, "docker", "image", "inspect", "--format", format, image.Ref)
	switch {
	case err != nil && strings.Contains(err.Error(), "No such image"):
		image.Missing = "not built on this machine"
		return image
	case err != nil:
		image.Missing = "unreadable: " + err.Error()
		return image
	}
	image.ID, image.Label, _ = strings.Cut(found, " ")
	if image.Label == "<no value>" {
		image.Label = ""
	}
	return image
}

func machine(ctx context.Context, run lookup) string {
	docker, err := run(ctx, "docker", "version", "--format", "{{.Server.Version}} {{.Server.Os}}/{{.Server.Arch}}")
	if err != nil {
		docker = "unreadable: " + err.Error()
	}
	return fmt.Sprintf("%s/%s, %d CPUs, docker %s", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), docker)
}

// command runs one lookup and returns its trimmed output, or the first line of
// what it said on failure.
func command(ctx context.Context, name string, args ...string) (string, error) {
	// #nosec G204 -- fixed programs; the arguments are the lab's own pins and paths.
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
