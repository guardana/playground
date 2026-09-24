//go:build docsfrontmatter

package docscheck_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/guardana/playground/internal/docscheck/docsconfig"
	"github.com/guardana/playground/internal/docscheck/pages"
	"github.com/guardana/playground/internal/docscheck/repofiles"
)

// Every page under docs/ against docs/docs.json. It stays out of the gate
// until the existing pages carry their frontmatter; `make docs-frontmatter`
// runs it.
func TestEveryPageCarriesItsFrontmatter(t *testing.T) {
	files, err := repofiles.List(context.Background(), repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	tracked, err := repofiles.Tracked(context.Background(), repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, docsconfig.Path))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := docsconfig.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	result, err := pages.Check(os.DirFS(repoRoot), pages.Files{Listed: files, Tracked: tracked}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range result.Problems {
		t.Error(p)
	}
	t.Logf("%d pages and %d READMEs judged, %d problems", result.Pages, result.Readmes, len(result.Problems))
}
