//go:build ignore

// Command gen-docs-index writes docs/README.md, the map of the documentation,
// from the frontmatter of every page the repository tracks under docs/, so a
// draft nobody committed never reaches the map.
//
//	go run scripts/gen-docs-index.go                     # standard output
//	go run scripts/gen-docs-index.go -o docs/README.md   # the page
//
// Everything it decides lives in internal/docscheck/indexdoc, which the gate
// compiles and tests, and TestTheIndexIsCurrent fails when the committed page
// differs from what this writes. It runs from the repository root.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/guardana/playground/internal/docscheck/docsconfig"
	"github.com/guardana/playground/internal/docscheck/indexdoc"
	"github.com/guardana/playground/internal/docscheck/repofiles"
)

func main() {
	out := flag.String("o", "", "write the page to this file instead of standard output")
	flag.Parse()
	if flag.NArg() != 0 {
		fail(fmt.Errorf("unexpected argument %q", flag.Arg(0)))
	}
	page, err := render()
	if err != nil {
		fail(err)
	}
	if *out == "" {
		_, err = os.Stdout.Write(page)
	} else {
		err = os.WriteFile(*out, page, 0o644) // #nosec G306 -- a tracked documentation page.
	}
	if err != nil {
		fail(err)
	}
}

func render() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	files, err := repofiles.Tracked(ctx, ".")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(docsconfig.Path)
	if err != nil {
		return nil, err
	}
	cfg, err := docsconfig.Parse(data)
	if err != nil {
		return nil, err
	}
	found, err := indexdoc.Collect(os.DirFS("."), files, cfg)
	if err != nil {
		return nil, err
	}
	return indexdoc.Render(cfg, found.Listed)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "gen-docs-index: %v\n", err)
	os.Exit(1)
}
