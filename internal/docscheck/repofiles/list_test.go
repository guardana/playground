package repofiles_test

import (
	"context"
	"errors"
	"testing"

	"github.com/guardana/playground/internal/docscheck/repofiles"
)

func TestParseRefusesWhatIsNotACleanPath(t *testing.T) {
	for _, out := range []string{"", "\n", "a.md\n\nb.md\n", "/etc/passwd\n", "../up.md\n", "docs/./a.md\n"} {
		if _, err := repofiles.Parse([]byte(out)); !errors.Is(err, repofiles.ErrInvalid) {
			t.Errorf("Parse(%q) = %v, want a refusal", out, err)
		}
	}
}

func TestParseReadsOnePathPerLine(t *testing.T) {
	got, err := repofiles.Parse([]byte("README.md\ndocs/status.md\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "README.md" || got[1] != "docs/status.md" {
		t.Errorf("Parse = %q", got)
	}
}

func TestListReadsThisRepository(t *testing.T) {
	files, err := repofiles.List(context.Background(), "../../..")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range files {
		if f == "go.mod" {
			found = true
		}
	}
	if !found {
		t.Errorf("the list of %d files holds no go.mod", len(files))
	}
}
