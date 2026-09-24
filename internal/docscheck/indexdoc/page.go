// Package indexdoc renders docs/README.md, the map of the documentation,
// from each page's own frontmatter, so the map cannot name a page that does
// not exist or describe one differently from the page itself.
package indexdoc

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/docscheck/docsconfig"
	"github.com/guardana/playground/internal/docscheck/frontmatter"
)

// Script is the generator the index names in its frontmatter.
const Script = "scripts/gen-docs-index.go"

// Index is where the page lives, relative to the repository.
const Index = "docs/README.md"

// Page is one entry of the map.
type Page struct {
	Path string
	Meta frontmatter.Meta
}

// Pages is what a collection found: the pages the map lists and how many
// pages under docs/ were read to find them.
type Pages struct {
	Listed   []Page
	Examined int
}

// ErrIndex is wrapped by every refusal.
var ErrIndex = errors.New("indexdoc")

// Collect reads every page under docs/ that files lists and the
// configuration does not exclude, but the index itself. A page with no block
// is left out of the map; a page whose block does not parse is a refusal,
// never a page left out.
func Collect(fsys fs.FS, files []string, cfg docsconfig.Config) (Pages, error) {
	var found Pages
	for _, rel := range files {
		if !strings.HasPrefix(rel, "docs/") || !strings.HasSuffix(rel, ".md") || rel == Index || cfg.Excludes(rel) {
			continue
		}
		found.Examined++
		data, err := fs.ReadFile(fsys, rel)
		if err != nil {
			return Pages{}, fmt.Errorf("%w: %w", ErrIndex, err)
		}
		if !frontmatter.HasBlock(data) {
			continue
		}
		meta, _, err := frontmatter.Parse(data)
		if err != nil {
			return Pages{}, fmt.Errorf("%w: %s: %w", ErrIndex, rel, err)
		}
		found.Listed = append(found.Listed, Page{Path: rel, Meta: meta})
	}
	if found.Examined == 0 {
		return Pages{}, fmt.Errorf("%w: no page under docs/, so the map would examine nothing", ErrIndex)
	}
	return found, nil
}

// Render renders the index: its own block, then a section per declared type
// that has a page, each page a link with its summary, in path order.
func Render(cfg docsconfig.Config, pages []Page) ([]byte, error) {
	head, err := frontmatter.Render(frontmatter.Meta{
		Title:     "Documentation",
		Summary:   "Every page under docs, grouped by type, rendered from each page's own frontmatter.",
		Type:      "project",
		Audience:  slices.Clone(cfg.Audiences),
		Covers:    []string{"docs/**"},
		Generated: Script,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIndex, err)
	}
	var b strings.Builder
	b.Write(head)
	b.WriteString("\n# Documentation\n\nRendered from every page's own frontmatter by `" + Script + "`. Rebuild it\nwith `make docs-gen`; an edit made here does not survive the next run.\n")
	sorted := slices.Clone(pages)
	slices.SortFunc(sorted, func(a, b Page) int { return strings.Compare(a.Path, b.Path) })
	listed := 0
	for _, typ := range cfg.Types {
		section := slices.DeleteFunc(slices.Clone(sorted), func(p Page) bool { return p.Meta.Type != typ.Name })
		if len(section) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n", typ.Heading)
		for _, p := range section {
			fmt.Fprintf(&b, "- [%s](%s): %s\n", p.Meta.Title, strings.TrimPrefix(p.Path, "docs/"), p.Meta.Summary)
		}
		listed += len(section)
	}
	if listed != len(pages) {
		return nil, fmt.Errorf("%w: %d of %d pages declare a type the configuration does not", ErrIndex, len(pages)-listed, len(pages))
	}
	if listed == 0 {
		b.WriteString("\nNo other page carries frontmatter yet.\n")
	}
	return []byte(b.String()), nil
}
