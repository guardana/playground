package indexdoc_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/guardana/playground/internal/docscheck/docsconfig"
	"github.com/guardana/playground/internal/docscheck/indexdoc"
)

const config = `{
  "types": [
    {"name": "runbook", "heading": "Runbooks", "budget": 1200},
    {"name": "project", "heading": "Project", "exempt": true}
  ],
  "audiences": ["engineering", "product"],
  "readme": {"root": 600, "folder": 300},
  "ceilings": {},
  "directories": {"docs": "project", "docs/runbooks": "runbook"},
  "page_types": {},
  "frozen": [],
  "excluded": ["docs/old/"],
  "surfaces": ["runner/**"]
}`

func block(title, summary, typ string) string {
	return "---\ntitle: " + title + "\nsummary: " + summary + "\ntype: " + typ +
		"\naudience: [engineering]\ncovers: [runner/**]\n---\n\n# " + title + "\n"
}

func collect(t *testing.T, fsys fstest.MapFS) (docsconfig.Config, indexdoc.Pages, error) {
	t.Helper()
	cfg, err := docsconfig.Parse([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for name := range fsys {
		files = append(files, name)
	}
	found, err := indexdoc.Collect(fsys, files, cfg)
	return cfg, found, err
}

func TestTheIndexListsPagesWithABlockByType(t *testing.T) {
	cfg, found, err := collect(t, fstest.MapFS{
		"docs/runbooks/run.md": {Data: []byte(block("Run a scenario", "Run one.", "runbook"))},
		"docs/status.md":       {Data: []byte(block("Status", "What exists.", "project"))},
		"docs/lab-files.md":    {Data: []byte("# Lab files\n")},
		"docs/old/gone.md":     {Data: []byte("---\nbroken\n")},
		"docs/README.md":       {Data: []byte("stale\n")},
		"README.md":            {Data: []byte("# Lab\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if found.Examined != 3 || len(found.Listed) != 2 {
		t.Fatalf("examined %d and listed %d, want 3 and 2", found.Examined, len(found.Listed))
	}
	page, err := indexdoc.Render(cfg, found.Listed)
	if err != nil {
		t.Fatal(err)
	}
	want := "---\ntitle: Documentation\n" +
		"summary: Every page under docs, grouped by type, rendered from each page's own frontmatter.\n" +
		"type: project\naudience: [engineering, product]\ncovers: [docs/**]\ngenerated: scripts/gen-docs-index.go\n---\n\n" +
		"# Documentation\n\nRendered from every page's own frontmatter by `scripts/gen-docs-index.go`. Rebuild it\n" +
		"with `make docs-gen`; an edit made here does not survive the next run.\n" +
		"\n## Runbooks\n\n- [Run a scenario](runbooks/run.md): Run one.\n" +
		"\n## Project\n\n- [Status](status.md): What exists.\n"
	if string(page) != want {
		t.Errorf("Render =\n%s\nwant\n%s", page, want)
	}
}

func TestAnIndexOfNoPageSaysSo(t *testing.T) {
	cfg, found, err := collect(t, fstest.MapFS{"docs/status.md": {Data: []byte("# Status\n")}})
	if err != nil {
		t.Fatal(err)
	}
	page, err := indexdoc.Render(cfg, found.Listed)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(page), "\nNo other page carries frontmatter yet.\n") || strings.Contains(string(page), "## ") {
		t.Errorf("an index of no page:\n%s", page)
	}
}

func TestABrokenBlockIsRefusedNotLeftOut(t *testing.T) {
	_, _, err := collect(t, fstest.MapFS{"docs/status.md": {Data: []byte("---\ntitle: Status\n---\n\n# Status\n")}})
	if !errors.Is(err, indexdoc.ErrIndex) || !strings.Contains(err.Error(), "docs/status.md") {
		t.Errorf("Collect = %v, want a refusal naming the page", err)
	}
}

func TestAnIndexOverNoPageIsRefused(t *testing.T) {
	_, _, err := collect(t, fstest.MapFS{"README.md": {Data: []byte("# Lab\n")}})
	if !errors.Is(err, indexdoc.ErrIndex) {
		t.Errorf("Collect = %v, want a refusal", err)
	}
}

func TestAPageOfAnUndeclaredTypeIsRefused(t *testing.T) {
	cfg, found, err := collect(t, fstest.MapFS{"docs/status.md": {Data: []byte(block("Status", "What exists.", "tutorial"))}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := indexdoc.Render(cfg, found.Listed); !errors.Is(err, indexdoc.ErrIndex) {
		t.Errorf("Render = %v, want a refusal", err)
	}
}
