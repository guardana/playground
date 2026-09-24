//go:build ignore

// Command docs-impact names the documentation pages a change makes suspect:
// the pages whose covers globs match a changed path, the changed paths under
// a surface no page covers, and those under a frozen path.
//
//	make docs-impact RANGE=main..HEAD   # the pages a range of commits makes suspect
//	make docs-impact FOR=runner/lab.go  # the pages covering one path
//	make docs-impact STALE=1            # commits since each page last moved
//
// Everything it does lives in internal/docscheck/impact, which the gate
// compiles and tests. It runs from the repository root. The binary exits 1
// when a page did not parse and is missing from the lists, and 2 when the run
// could not be made, including anything git could not measure; `go run` and
// `make` both fold these into one failure, so the printed NOT MEASURED line is
// what tells them apart there.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/guardana/playground/internal/docscheck/impact"
	"github.com/guardana/playground/internal/docscheck/repofiles"
)

func main() {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "docs-impact: %v\n", err)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	seams := impact.Seams{
		FS:    os.DirFS(wd),
		Files: func(ctx context.Context) ([]string, error) { return repofiles.List(ctx, wd) },
		Git:   impact.Repository(impact.Git(wd, os.Environ()), wd),
	}
	code := impact.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, seams)
	cancel()
	os.Exit(code)
}
