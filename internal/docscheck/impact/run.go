package impact

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/guardana/playground/internal/docscheck/docsconfig"
	"github.com/guardana/playground/internal/docscheck/frontmatter"
)

// Seams is what one run reads: the repository as a file system, the list of
// files it owns, and git bound to its working directory.
type Seams struct {
	FS    fs.FS
	Files func(context.Context) ([]string, error)
	Git   Runner
}

type options struct {
	rng, forPath string
	stale        bool
}

// Run is the docs-impact program over its seams. It returns the exit status:
// 0, 1 when a page did not parse and is missing from the lists, 2 when the
// run could not be made, a result git could not measure among them.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, s Seams) int {
	o, err := parseArgs(args, stderr)
	if err == nil {
		var code int
		if code, err = execute(ctx, o, stdout, s); err == nil {
			return code
		}
	}
	_, _ = io.WriteString(stderr, "docs-impact: "+err.Error()+"\n")
	return 2
}

func parseArgs(args []string, stderr io.Writer) (options, error) {
	var o options
	set := flag.NewFlagSet("docs-impact", flag.ContinueOnError)
	set.SetOutput(stderr)
	set.StringVar(&o.rng, "range", "", "report on the paths `git diff --name-only <range>` lists")
	set.StringVar(&o.forPath, "for", "", "print the pages covering this path")
	set.BoolVar(&o.stale, "stale", false, "per page, the commits touching its covers since the page's last commit")
	if err := set.Parse(args); err != nil {
		return options{}, err
	}
	if set.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected argument %q", set.Arg(0))
	}
	modes := 0
	for _, on := range []bool{o.rng != "", o.forPath != "", o.stale} {
		if on {
			modes++
		}
	}
	if modes != 1 {
		return options{}, errors.New("exactly one of --range, --for and --stale is required")
	}
	return o, nil
}

func execute(ctx context.Context, o options, stdout io.Writer, s Seams) (int, error) {
	data, err := fs.ReadFile(s.FS, docsconfig.Path)
	if err != nil {
		return 0, fmt.Errorf("run from the repository root: %w", err)
	}
	cfg, err := docsconfig.Parse(data)
	if err != nil {
		return 0, err
	}
	files, err := s.Files(ctx)
	if err != nil {
		return 0, err
	}
	pages, broken, err := readPages(s.FS, files, cfg)
	if err != nil {
		return 0, err
	}
	var text string
	switch {
	case o.forPath != "":
		var covering []string
		covering, err = Covering(pages, o.forPath)
		text = fmt.Sprintf("%d pages cover %s of %d examined\n", len(covering), o.forPath, len(pages))
		for _, p := range covering {
			text += "  " + p + "\n"
		}
	case o.stale:
		var rows []Staleness
		rows, err = Stale(ctx, s.Git, pages)
		text = FormatStale(rows)
	default:
		text, err = rangeText(ctx, s.Git, cfg, pages, o.rng)
	}
	if err != nil {
		return 0, err
	}
	if _, err := io.WriteString(stdout, text+broken); err != nil {
		return 0, fmt.Errorf("writing to standard output: %w", err)
	}
	if broken != "" {
		return 1, nil
	}
	return 0, nil
}

func rangeText(ctx context.Context, git Runner, cfg docsconfig.Config, pages []Page, rng string) (string, error) {
	changed, err := Changed(ctx, git, rng)
	if err != nil {
		return "", err
	}
	r, err := Report(pages, cfg.Surfaces, cfg.Frozen, changed)
	if err != nil {
		return "", err
	}
	return r.String(), nil
}

// readPages parses every page under docs/ the file list holds. A page whose
// frontmatter does not parse is named after the lists, never dropped.
func readPages(fsys fs.FS, files []string, cfg docsconfig.Config) ([]Page, string, error) {
	var pages []Page
	var broken []string
	for _, rel := range files {
		if !strings.HasPrefix(rel, "docs/") || !strings.HasSuffix(rel, ".md") || cfg.Excludes(rel) {
			continue
		}
		data, err := fs.ReadFile(fsys, rel)
		if err != nil {
			return nil, "", err
		}
		meta, _, err := frontmatter.Parse(data)
		if err != nil {
			broken = append(broken, fmt.Sprintf("  %s: %v\n", rel, err))
			continue
		}
		pages = append(pages, Page{Path: rel, Covers: meta.Covers})
	}
	if len(pages) == 0 {
		return nil, "", fmt.Errorf("%w: none of the %d pages under docs parsed:\n%s", ErrInvalid, len(broken), strings.Join(broken, ""))
	}
	if len(broken) == 0 {
		return pages, "", nil
	}
	return pages, fmt.Sprintf("%d pages did not parse and are missing above\n%s", len(broken), strings.Join(broken, "")), nil
}
