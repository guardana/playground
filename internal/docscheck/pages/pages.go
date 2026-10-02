// Package pages judges the documentation against docs/docs.json: every page
// under docs/ carries a valid frontmatter block whose title is its one H1,
// whose type is its directory's, whose audiences and covers are declared and
// real, whose diagrams name their sources, and whose words fit its budget.
// README.md files outside docs/ are held to their own budget.
package pages

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/docscheck/docsconfig"
	"github.com/guardana/playground/internal/docscheck/frontmatter"
	"github.com/guardana/playground/internal/docscheck/glob"
	"github.com/guardana/playground/internal/docscheck/markdown"
)

// Problem is one finding against one file.
type Problem struct {
	Path   string
	Reason string
}

func (p Problem) String() string { return p.Path + ": " + p.Reason }

// Result is how many files were judged and what was wrong with them.
type Result struct {
	Pages    int
	Readmes  int
	Problems []Problem
}

// ErrNothing is returned when the file list holds no page: a check that
// judged nothing has not passed.
var ErrNothing = errors.New("pages: no page under docs/ to judge")

// Files is the repository's file list twice over. Listed chooses what is
// read and judged, so a page is checked before it is staged; Tracked is what
// a covers glob, a diagram source, a surface or a frozen pattern must name,
// because an untracked file is absent from every clone.
type Files struct {
	Listed, Tracked []string
}

// Check judges the listed files, read from fsys rooted at the repository.
func Check(fsys fs.FS, files Files, cfg docsconfig.Config) (Result, error) {
	var r Result
	for _, rel := range files.Listed {
		if !strings.HasSuffix(rel, ".md") || cfg.Excludes(rel) {
			continue
		}
		var problems []string
		var err error
		switch {
		case strings.HasPrefix(rel, "docs/"):
			r.Pages++
			var data []byte
			if data, err = fs.ReadFile(fsys, rel); err == nil {
				problems = judgePage(files, cfg, rel, data)
			}
		case path.Base(rel) == "README.md":
			r.Readmes++
			problems, err = judgeReadme(fsys, cfg, rel)
		default:
			continue
		}
		if err != nil {
			return Result{}, err
		}
		for _, p := range problems {
			r.Problems = append(r.Problems, Problem{Path: rel, Reason: p})
		}
	}
	if r.Pages == 0 {
		return Result{}, ErrNothing
	}
	for _, p := range configProblems(files, cfg) {
		r.Problems = append(r.Problems, Problem{Path: docsconfig.Path, Reason: p})
	}
	return r, nil
}

func judgePage(files Files, cfg docsconfig.Config, rel string, data []byte) []string {
	meta, body, err := frontmatter.Parse(data)
	if err != nil {
		return []string{err.Error()}
	}
	var problems []string
	problems = append(problems, titleProblems(meta, body)...)
	problems = append(problems, typeProblems(cfg, rel, meta)...)
	problems = append(problems, audienceProblems(cfg, meta)...)
	covers, coverProblems := compileCovers(files.Tracked, meta.Covers)
	problems = append(problems, coverProblems...)
	if meta.Generated != "" && !slices.Contains(files.Listed, meta.Generated) {
		problems = append(problems, fmt.Sprintf("generated names %s, which the repository does not hold", meta.Generated))
	}
	problems = append(problems, diagramProblems(files.Tracked, covers, body)...)
	if typ, ok := cfg.Lookup(meta.Type); ok && !typ.Exempt {
		problems = append(problems, budgetProblem(cfg, rel, frontmatter.Words(body), typ.Budget)...)
	}
	return problems
}

func titleProblems(meta frontmatter.Meta, body []byte) []string {
	headings := h1s(body)
	switch {
	case len(headings) == 0:
		return []string{"the page has no H1"}
	case len(headings) > 1:
		return []string{fmt.Sprintf("the page has %d H1 headings, one is allowed", len(headings))}
	case headings[0] != meta.Title:
		return []string{fmt.Sprintf("the title %q is not the H1 %q", meta.Title, headings[0])}
	}
	return nil
}

func h1s(body []byte) []string {
	var found []string
	for _, h := range markdown.Headings(body) {
		if h.Level == 1 {
			found = append(found, h.Text)
		}
	}
	return found
}

func typeProblems(cfg docsconfig.Config, rel string, meta frontmatter.Meta) []string {
	if _, ok := cfg.Lookup(meta.Type); !ok {
		return []string{fmt.Sprintf("type %q is not declared in %s", meta.Type, docsconfig.Path)}
	}
	want, ok := cfg.TypeOf(rel)
	switch {
	case !ok:
		return []string{fmt.Sprintf("the directory %s is not listed in %s", path.Dir(rel), docsconfig.Path)}
	case want != meta.Type:
		return []string{fmt.Sprintf("type %q, but a page here is %q", meta.Type, want)}
	}
	return nil
}

func audienceProblems(cfg docsconfig.Config, meta frontmatter.Meta) []string {
	var problems []string
	for _, a := range meta.Audience {
		if !slices.Contains(cfg.Audiences, a) {
			problems = append(problems, fmt.Sprintf("audience %q is not declared in %s", a, docsconfig.Path))
		}
	}
	return problems
}

// compileCovers compiles the page's globs and refuses one under which the
// repository tracks no file: it would keep the page from ever being suspect.
func compileCovers(files, covers []string) ([]glob.Glob, []string) {
	var globs []glob.Glob
	var problems []string
	for _, c := range covers {
		g, err := glob.Compile(c)
		if err != nil {
			problems = append(problems, fmt.Sprintf("covers: %v", err))
			continue
		}
		globs = append(globs, g)
		if !slices.ContainsFunc(files, g.Match) {
			problems = append(problems, fmt.Sprintf("covers %s matches no file the repository tracks", c))
		}
	}
	return globs, problems
}
