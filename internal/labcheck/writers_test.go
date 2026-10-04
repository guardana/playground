package labcheck_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

// `make up` creates one journal directory per writer, and compose refuses a
// writer whose directory is missing; a writer added to the lab and left out of
// the Makefile would not come up by hand.
func TestTheMakefileMakesADirectoryForEveryJournalWriter(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`(?m)^JOURNAL_WRITERS = (.*)$`).FindAllSubmatch(body, -1)
	if len(found) != 1 {
		t.Fatalf("Makefile: %d JOURNAL_WRITERS lines, want 1", len(found))
	}
	got := slices.Sorted(slices.Values(strings.Fields(string(found[0][1]))))
	want := slices.Sorted(slices.Values(append(labspec.Victims(), labspec.PDPDoubleJournal, labspec.ApproverJournal)))
	if !slices.Equal(got, want) {
		t.Errorf("Makefile JOURNAL_WRITERS names %v, want %v", got, want)
	}
}
