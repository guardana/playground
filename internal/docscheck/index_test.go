package docscheck_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/docscheck/docsconfig"
	"github.com/guardana/playground/internal/docscheck/indexdoc"
	"github.com/guardana/playground/internal/docscheck/repofiles"
)

// The map of the documentation is rendered here from every page's own
// frontmatter and must be the committed page, byte for byte.
func TestTheIndexIsCurrent(t *testing.T) {
	files, err := repofiles.Tracked(context.Background(), repoRoot)
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
	found, err := indexdoc.Collect(os.DirFS(repoRoot), files, cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := indexdoc.Render(cfg, found.Listed)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(repoRoot, indexdoc.Index))
	if err != nil {
		t.Fatalf("%v; generate it with `make docs-gen`", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is not what %s renders; rebuild it with `make docs-gen`\n%s", indexdoc.Index, indexdoc.Script, firstDifference(got, want))
	}
}

func firstDifference(got, want []byte) string {
	g, w := strings.Split(string(got), "\n"), strings.Split(string(want), "\n")
	for i := 0; i < len(g) || i < len(w); i++ {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl {
			return "line " + strconv.Itoa(i+1) + ":\n  rendered:  " + gl + "\n  committed: " + wl
		}
	}
	return ""
}
