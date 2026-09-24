// Package impact says which documentation pages a change makes suspect. It
// judges a list of changed paths against the covers globs of every page, the
// documentable surfaces and the frozen paths of docs/docs.json, and reads
// git's output for the paths a range changed and the commits that touched a
// page's covers since the page itself was last committed. A git that cannot
// answer, or answers for another repository, is NOT MEASURED, never an empty
// list.
package impact

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/docscheck/glob"
)

// Page is one documentation page and the covers globs it declares.
type Page struct {
	Path   string
	Covers []string
}

// Review is a page a change makes suspect, with the changed paths under its
// covers.
type Review struct {
	Page    string
	Changed []string
}

// Hit is a changed path and the glob that names it.
type Hit struct {
	Path string
	Glob string
}

// Result is what one run examined and what it found. Each list is printed
// with the count behind it, so an empty list says how much was looked at.
type Result struct {
	Pages, Surfaces, Frozen, Changed int
	Review                           []Review
	// Uncovered are changed paths under a surface that no page covers.
	Uncovered []Hit
	// Contract are changed paths under a frozen glob.
	Contract []Hit
}

// ErrInvalid is wrapped by every refusal of an input.
var ErrInvalid = errors.New("impact")

type page struct {
	path   string
	covers []glob.Glob
}

// Report judges the changed paths against the pages' covers, the surfaces and
// the frozen globs. A run with no page or no surface is refused: it would
// examine nothing and report that nothing is suspect.
func Report(pages []Page, surfaces, frozen, changed []string) (Result, error) {
	ps, err := compilePages(pages)
	if err != nil {
		return Result{}, err
	}
	if len(surfaces) == 0 {
		return Result{}, fmt.Errorf("%w: no surface to examine", ErrInvalid)
	}
	ss, err := compileAll(surfaces)
	if err != nil {
		return Result{}, fmt.Errorf("%w: surfaces: %w", ErrInvalid, err)
	}
	fs, err := compileAll(frozen)
	if err != nil {
		return Result{}, fmt.Errorf("%w: frozen: %w", ErrInvalid, err)
	}
	if err := checkChanged(changed); err != nil {
		return Result{}, err
	}
	r := Result{Pages: len(ps), Surfaces: len(ss), Frozen: len(fs), Changed: len(changed)}
	for _, p := range ps {
		if hits := matching(p.covers, changed); len(hits) > 0 {
			r.Review = append(r.Review, Review{Page: p.path, Changed: hits})
		}
	}
	for _, c := range changed {
		if g, ok := first(fs, c); ok {
			r.Contract = append(r.Contract, Hit{Path: c, Glob: g})
		}
		if g, ok := first(ss, c); ok && !slices.ContainsFunc(ps, func(p page) bool { return len(matching(p.covers, []string{c})) > 0 }) {
			r.Uncovered = append(r.Uncovered, Hit{Path: c, Glob: g})
		}
	}
	return r, nil
}

func checkChanged(changed []string) error {
	for i, c := range changed {
		if err := checkPath(c); err != nil {
			return fmt.Errorf("%w: changed: %w", ErrInvalid, err)
		}
		if slices.Contains(changed[:i], c) {
			return fmt.Errorf("%w: changed: %s is listed twice", ErrInvalid, c)
		}
	}
	return nil
}

// Covering names the pages whose covers hold one path.
func Covering(pages []Page, p string) ([]string, error) {
	ps, err := compilePages(pages)
	if err != nil {
		return nil, err
	}
	if err := checkPath(p); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	var out []string
	for _, pg := range ps {
		if len(matching(pg.covers, []string{p})) > 0 {
			out = append(out, pg.path)
		}
	}
	return out, nil
}

// String spells the lists, each with the counts behind it. The first line
// is how many changed paths were read, so a range that changed nothing shows
// as zero read, never as nothing to review.
func (r Result) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d changed paths read\n", r.Changed)
	fmt.Fprintf(&b, "%d pages to review of %d examined\n", len(r.Review), r.Pages)
	for _, rv := range r.Review {
		fmt.Fprintf(&b, "  %s <- %s\n", rv.Page, strings.Join(rv.Changed, ", "))
	}
	fmt.Fprintf(&b, "%d changed paths under a surface no page covers, of %d surfaces examined\n", len(r.Uncovered), r.Surfaces)
	for _, h := range r.Uncovered {
		fmt.Fprintf(&b, "  %s (%s)\n", h.Path, h.Glob)
	}
	fmt.Fprintf(&b, "%d changed paths under a frozen path, a contract change, of %d frozen globs examined\n", len(r.Contract), r.Frozen)
	for _, h := range r.Contract {
		fmt.Fprintf(&b, "  %s (%s)\n", h.Path, h.Glob)
	}
	return b.String()
}

func compilePages(pages []Page) ([]page, error) {
	if len(pages) == 0 {
		return nil, fmt.Errorf("%w: no page to examine", ErrInvalid)
	}
	out := make([]page, 0, len(pages))
	for i, p := range pages {
		if p.Path == "" {
			return nil, fmt.Errorf("%w: page %d has no path", ErrInvalid, i)
		}
		if slices.ContainsFunc(out, func(q page) bool { return q.path == p.Path }) {
			return nil, fmt.Errorf("%w: %s is listed twice", ErrInvalid, p.Path)
		}
		globs, err := compileAll(p.Covers)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: covers: %w", ErrInvalid, p.Path, err)
		}
		out = append(out, page{path: p.Path, covers: globs})
	}
	return out, nil
}

func compileAll(patterns []string) ([]glob.Glob, error) {
	globs := make([]glob.Glob, 0, len(patterns))
	for _, p := range patterns {
		g, err := glob.Compile(p)
		if err != nil {
			return nil, err
		}
		globs = append(globs, g)
	}
	return globs, nil
}

func first(globs []glob.Glob, p string) (string, bool) {
	i := slices.IndexFunc(globs, func(g glob.Glob) bool { return g.Match(p) })
	if i < 0 {
		return "", false
	}
	return globs[i].String(), true
}

func matching(globs []glob.Glob, paths []string) []string {
	var hits []string
	for _, p := range paths {
		if slices.ContainsFunc(globs, func(g glob.Glob) bool { return g.Match(p) }) {
			hits = append(hits, p)
		}
	}
	return hits
}
