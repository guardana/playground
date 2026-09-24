package pages_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/guardana/playground/internal/docscheck/docsconfig"
	"github.com/guardana/playground/internal/docscheck/pages"
)

const config = `{
  "types": [
    {"name": "runbook", "heading": "Runbooks", "budget": 40},
    {"name": "project", "heading": "Project", "exempt": true}
  ],
  "audiences": ["engineering", "product"],
  "readme": {"root": 12, "folder": 6},
  "ceilings": {"victims/shell/README.md": 8},
  "directories": {"docs": "project", "docs/runbooks": "runbook"},
  "page_types": {},
  "frozen": ["docs/status.md"],
  "excluded": ["CHANGELOG.md"],
  "surfaces": ["runner/**"]
}`

const good = "---\n" +
	"title: Run a scenario\n" +
	"summary: Run one scenario and read the report.\n" +
	"type: runbook\n" +
	"audience: [engineering]\n" +
	"covers: [runner/**, compose/compose.yaml]\n" +
	"---\n\n" +
	"# Run a scenario\n\n" +
	"```mermaid\nflowchart LR\n  a --> b\n```\n\n" +
	"Sources: `runner/lab.go`,\n`compose/compose.yaml`.\n\n" +
	"Run it and read the report.\n"

const page = "docs/runbooks/run.md"

func tree(edits map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{
		page:                      {Data: []byte(good)},
		"docs/status.md":          {Data: []byte("---\ntitle: Status\nsummary: What exists.\ntype: project\naudience: [product]\ncovers: [runner/**]\n---\n\n# Status\n")},
		"runner/lab.go":           {Data: []byte("package runner\n")},
		"compose/compose.yaml":    {Data: []byte("services: {}\n")},
		"README.md":               {Data: []byte("# Lab\n\nTen words fit under the root budget of twelve.\n")},
		"victims/shell/README.md": {Data: []byte("# Shell\n\nSeven words, over six, under eight.\n")},
		"CHANGELOG.md":            {Data: []byte("# Changelog\n\nExcluded, so its many words are never counted against anything.\n")},
	}
	for name, data := range edits {
		fsys[name] = &fstest.MapFile{Data: []byte(data)}
	}
	return fsys
}

func check(t *testing.T, fsys fstest.MapFS) pages.Result {
	t.Helper()
	return checkUntracked(t, fsys)
}

// checkUntracked lists every file of fsys and tracks all but the untracked.
func checkUntracked(t *testing.T, fsys fstest.MapFS, untracked ...string) pages.Result {
	t.Helper()
	cfg, err := docsconfig.Parse([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	var files pages.Files
	for name := range fsys {
		files.Listed = append(files.Listed, name)
		if !slices.Contains(untracked, name) {
			files.Tracked = append(files.Tracked, name)
		}
	}
	r, err := pages.Check(fsys, files, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAGoodTreeHasNoProblem(t *testing.T) {
	r := check(t, tree(nil))
	if len(r.Problems) != 0 {
		t.Errorf("problems on a good tree: %v", r.Problems)
	}
	if r.Pages != 2 || r.Readmes != 2 {
		t.Errorf("judged %d pages and %d READMEs, want 2 and 2", r.Pages, r.Readmes)
	}
}

func TestEachRuleRefusesItsViolation(t *testing.T) {
	edit := func(from, to string) map[string]string {
		return map[string]string{page: strings.Replace(good, from, to, 1)}
	}
	words := strings.Repeat("word ", 40)
	cases := map[string]struct {
		edits      map[string]string
		path, want string
	}{
		"no frontmatter":            {map[string]string{page: "# Run a scenario\n"}, page, "line 1 is not"},
		"title is not the H1":       {edit("# Run a scenario", "# Run one scenario"), page, "is not the H1"},
		"no H1":                     {edit("# Run a scenario", "## Run a scenario"), page, "has no H1"},
		"two H1":                    {edit("Run it and", "# Run a scenario\n\nRun it and"), page, "2 H1"},
		"an undeclared type":        {edit("type: runbook", "type: tutorial"), page, "not declared"},
		"the directory's type":      {edit("type: runbook", "type: project"), page, "a page here is \"runbook\""},
		"an unlisted directory":     {map[string]string{"docs/elsewhere/run.md": good}, "docs/elsewhere/run.md", "is not listed"},
		"an undeclared audience":    {edit("audience: [engineering]", "audience: [marketing]"), page, "audience \"marketing\""},
		"a dead covers glob":        {edit("covers: [runner/**, compose/compose.yaml]", "covers: [runner/**, compose/compose.yaml, gone/**]"), page, "gone/** matches no file"},
		"a diagram with no sources": {edit("Sources: `runner/lab.go`,\n`compose/compose.yaml`.", "Drawn by hand."), page, "not a Sources: line"},
		"a source not held":         {edit("`runner/lab.go`", "`runner/gone.go`"), page, "runner/gone.go is not a file"},
		"a source outside covers":   {map[string]string{page: strings.Replace(good, "`runner/lab.go`", "`README.md`", 1)}, page, "README.md is outside the page's covers"},
		"an unbackticked source":    {edit("`runner/lab.go`", "runner/lab.go"), page, "not one backticked path"},
		"a page over its budget":    {edit("Run it and", words+"Run it and"), page, "over the budget of 40"},
		"a root README over budget": {map[string]string{"README.md": "# Lab\n\n" + words}, "README.md", "over the budget of 12"},
		"a README over its ceiling": {map[string]string{"victims/shell/README.md": "# Shell\n\none two three four five six seven eight\n"}, "victims/shell/README.md", "over its ceiling of 8"},
		"a folder README over":      {map[string]string{"attacks/README.md": "# Attacks\n\none two three four five six\n"}, "attacks/README.md", "over the budget of 6"},
		"a generator not held":      {edit("covers: [runner/**, compose/compose.yaml]\n", "covers: [runner/**, compose/compose.yaml]\ngenerated: scripts/gen-docs-index.go\n"), page, "scripts/gen-docs-index.go, which"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := check(t, tree(c.edits))
			for _, p := range r.Problems {
				if p.Path == c.path && strings.Contains(p.Reason, c.want) {
					return
				}
			}
			t.Errorf("no problem on %s saying %q; got %v", c.path, c.want, r.Problems)
		})
	}
}

func TestTheConfigurationNamesRealFiles(t *testing.T) {
	fsys := tree(nil)
	delete(fsys, "runner/lab.go")
	delete(fsys, "victims/shell/README.md")
	fsys[page] = &fstest.MapFile{Data: []byte(strings.Replace(good, "`runner/lab.go`,\n", "", 1))}
	fsys["docs/status.md"] = &fstest.MapFile{Data: []byte(strings.Replace(string(fsys["docs/status.md"].Data), "runner/**", "compose/**", 1))}
	r := check(t, fsys)
	for _, want := range []string{"surfaces runner/** matches no file", "ceilings name victims/shell/README.md"} {
		found := false
		for _, p := range r.Problems {
			found = found || p.Path == "docs/docs.json" && strings.Contains(p.Reason, want)
		}
		if !found {
			t.Errorf("no problem on docs/docs.json saying %q; got %v", want, r.Problems)
		}
	}
}

func TestACheckThatJudgedNoPageFails(t *testing.T) {
	cfg, err := docsconfig.Parse([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{"README.md": {Data: []byte("# Lab\n")}}
	if _, err := pages.Check(fsys, pages.Files{Listed: []string{"README.md"}, Tracked: []string{"README.md"}}, cfg); !errors.Is(err, pages.ErrNothing) {
		t.Errorf("Check = %v, want ErrNothing", err)
	}
}

// An untracked file is absent from a clone: a covers glob, a diagram source or
// a surface naming only one is refused, while an untracked page is judged.
func TestOnlyATrackedFileCounts(t *testing.T) {
	draft := "docs/runbooks/draft.md"
	r := checkUntracked(t, tree(map[string]string{draft: "# Draft\n"}), "runner/lab.go", draft)
	for _, want := range []struct{ path, reason string }{
		{page, "covers runner/** matches no file"},
		{page, "source runner/lab.go is not a file"},
		{"docs/status.md", "covers runner/** matches no file"},
		{"docs/docs.json", "surfaces runner/** matches no file"},
		{draft, "line 1 is not"},
	} {
		if !slices.ContainsFunc(r.Problems, func(p pages.Problem) bool {
			return p.Path == want.path && strings.Contains(p.Reason, want.reason)
		}) {
			t.Errorf("no problem on %s saying %q; got %v", want.path, want.reason, r.Problems)
		}
	}
	if r.Pages != 3 {
		t.Errorf("judged %d pages, want 3 with the untracked draft", r.Pages)
	}
}
