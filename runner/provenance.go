package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/guardana/playground/internal/hostpath"
	"github.com/guardana/playground/runner/driver"
	"github.com/guardana/playground/runner/report"
)

// provenanceTimeout bounds the git and docker lookups a report header needs.
const provenanceTimeout = 30 * time.Second

// lookup runs one program and returns its trimmed output; command is the real
// one, and a test hands in what the program would have said.
type lookup = driver.Lookup

// describeHost reads what produced a run from the lab's checkout, the
// workspace, versions.env, the local Docker daemon, the runtime and each
// subject's own account of its build.
func describeHost(root string, space workspace, run lookup, subjects []driver.Subject) func(context.Context) report.Provenance {
	return func(ctx context.Context) report.Provenance {
		ctx, cancel := context.WithTimeout(ctx, provenanceTimeout)
		defer cancel()
		described := report.Provenance{
			Lab: labCommit(ctx, run, root), Workspace: describeWorkspace(ctx, run, space), Machine: machine(ctx, run),
		}
		pins, err := driver.ReadPins(filepath.Join(root, versionFile))
		said := describeSubjects(ctx, subjects, run, pins)
		described.Development = said.Development
		if err != nil {
			described.Pins = []report.Pin{{Name: versionFile, Value: "unreadable: " + err.Error()}}
			return described
		}
		described.Pins = pins
		described.Images = said.Images
		return described
	}
}

// describeSubjects joins what every subject says of its build, in the order
// the subjects are listed.
func describeSubjects(ctx context.Context, subjects []driver.Subject, run lookup, pins driver.Pins) driver.Description {
	var joined driver.Description
	var development []string
	for _, subject := range subjects {
		said := subject.Describe(ctx, run, pins)
		joined.Images = append(joined.Images, said.Images...)
		if said.Development != "" {
			development = append(development, said.Development)
		}
	}
	joined.Development = strings.Join(development, "; ")
	return joined
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
	case hostpath.Resolved(top) != hostpath.Resolved(root):
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

func machine(ctx context.Context, run lookup) string {
	docker, err := run(ctx, "docker", "version", "--format", "{{.Server.Version}} {{.Server.Os}}/{{.Server.Arch}}")
	if err != nil {
		docker = "unreadable: " + err.Error()
	}
	return fmt.Sprintf("%s/%s, %d CPUs, docker %s", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), docker)
}
