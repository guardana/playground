package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/guardana/playground/internal/assertion"
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

// describeHost reads what produced a run from the lab's checkout, the
// workspace, versions.env, the local Docker daemon and the runtime.
func describeHost(root string, space workspace, run lookup, dev *devBuild) func(context.Context) report.Provenance {
	return func(ctx context.Context) report.Provenance {
		ctx, cancel := context.WithTimeout(ctx, provenanceTimeout)
		defer cancel()
		described := report.Provenance{
			Lab: labCommit(ctx, run, root), Workspace: describeWorkspace(ctx, run, space), Machine: machine(ctx, run),
		}
		pins, err := readPins(filepath.Join(root, versionFile))
		if dev != nil {
			described.Development = dev.describe(pinValue(pins, "ENFORCER_COMMIT"))
		}
		if err != nil {
			described.Pins = []report.Pin{{Name: versionFile, Value: "unreadable: " + err.Error()}}
			return described
		}
		described.Pins = pins
		described.Images = images(ctx, run, pins)
		return described
	}
}

// images reads the enforcer by the commit and the tree its build stamps and
// the verifier by the release its build stamps.
func images(ctx context.Context, run lookup, pins []report.Pin) []report.Image {
	name, commit := pinValue(pins, "ENFORCER_IMAGE"), pinValue(pins, "ENFORCER_COMMIT")
	enforcer := inspectImage(ctx, run, name, commit, revisionLabel)
	enforcer.TreeWant = or(pinValue(pins, "ENFORCER_TREE"), "none in versions.env")
	if enforcer.Missing == "" {
		tree := inspectImage(ctx, run, name, commit, treeLabel)
		enforcer.Tree = or(tree.Missing, tree.Label)
	}
	return []report.Image{
		enforcer,
		inspectImage(ctx, run, pinValue(pins, "VERIFIER_IMAGE"), pinValue(pins, "VERIFIER_VERSION"), versionLabel),
	}
}

func or(value, otherwise string) string {
	if value == "" {
		return otherwise
	}
	return value
}

// refuseVerifierImage refuses a run the verifier grades when the local image
// under the pin's tag was not built from VERIFIER_VERSION: compose never pulls
// it, and a tag alone says nothing about what was installed behind it.
func (l lab) refuseVerifierImage(ctx context.Context) error {
	if l.inspect == nil {
		return errors.New("nothing reads the verifier's image")
	}
	pins, err := readPins(filepath.Join(l.root, versionFile))
	if err != nil {
		return fmt.Errorf("the pins cannot be read: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, provenanceTimeout)
	defer cancel()
	want := pinValue(pins, "VERIFIER_VERSION")
	image := inspectImage(ctx, l.inspect, pinValue(pins, "VERIFIER_IMAGE"), want, versionLabel)
	switch {
	case image.Missing != "":
		return fmt.Errorf("%s: %s", image.Ref, image.Missing)
	case image.Label != want:
		return fmt.Errorf("%s carries %s %q, want %q", image.Ref, versionLabel, image.Label, want)
	}
	return nil
}

// imageRefused is the report of a run refused over the verifier's image.
func imageRefused(id, runID string, cause error, at time.Time) assertion.Report {
	return assertion.Report{
		Scenario: id, RunID: runID, StartedAt: at, EndedAt: at,
		Results: []assertion.Result{{
			Check: "verifier/image", Outcome: assertion.Fail,
			Want: "the verifier's image built from VERIFIER_VERSION",
			Got:  "the run was refused before anything was brought up", Detail: cause.Error(),
		}},
	}
}

func (l lab) provenance(ctx context.Context) report.Provenance {
	if l.describe == nil {
		return report.Provenance{}
	}
	return l.describe(ctx)
}

// describeWorkspace names the workspace by its path and, when it is a
// checkout, its commit, which is what a person rerunning the scenario needs.
func describeWorkspace(ctx context.Context, run lookup, space workspace) string {
	if !space.external {
		return "the clone"
	}
	return fmt.Sprintf("`%s`, %s", space.dir, labCommit(ctx, run, space.dir))
}

// labCommit names the commit of the checkout rooted at root. git answers from
// the nearest enclosing repository, so a directory that is not the top of one
// is not reported under that repository's commit.
func labCommit(ctx context.Context, run lookup, root string) string {
	top, err := run(ctx, "git", "-C", root, "rev-parse", "--show-toplevel")
	switch {
	case err != nil:
		return "not a git checkout: " + err.Error()
	case resolved(top) != resolved(root):
		return fmt.Sprintf("not a git checkout (inside %s)", top)
	}
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
