package labcheck_test

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

// Every persona a scenario names exists, and every persona in the directory is
// named by one: the double answers 404 for a persona that is not there, and one
// nobody names is a metadata document nothing tests.
func TestEveryAuthPersonaIsNamedAndExists(t *testing.T) {
	scenarios := filesUnder(t, "scenarios")
	if len(scenarios) == 0 {
		t.Fatal("no scenarios found; this guard inspected nothing")
	}
	var named []string
	for _, path := range scenarios {
		scenario, err := labspec.LoadScenario(path)
		if err != nil || scenario.AuthDouble == "" {
			continue
		}
		file := labspec.AuthPersonaDir + scenario.AuthDouble + ".json"
		named = append(named, file)
		if _, err := os.Stat(filepath.Join(repoRoot, file)); err != nil {
			t.Errorf("%s names persona %s, which does not exist", relative(path), file)
		}
	}
	entries, err := os.ReadDir(filepath.Join(repoRoot, labspec.AuthPersonaDir))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		file := labspec.AuthPersonaDir + entry.Name()
		if !strings.HasSuffix(file, ".json") || !slices.Contains(named, file) {
			t.Errorf("%s is named by no scenario", file)
		}
	}
}

// A persona sends the verifier only to the double: a URL naming any other host
// would have it fetch from somewhere the lab does not run.
func TestEveryPersonaURLNamesTheDouble(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join(repoRoot, labspec.AuthPersonaDir))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		body, err := os.ReadFile(filepath.Join(repoRoot, labspec.AuthPersonaDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var document any
		if err := json.Unmarshal(body, &document); err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
		for _, value := range stringsIn(document) {
			if !strings.Contains(value, "://") {
				continue
			}
			if parsed, err := url.Parse(value); err != nil || parsed.Hostname() != labspec.AuthDoubleJournal {
				t.Errorf("%s names %q, want a URL on %s", entry.Name(), value, labspec.AuthDoubleJournal)
			}
		}
	}
}

func stringsIn(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		var found []string
		for _, item := range typed {
			found = append(found, stringsIn(item)...)
		}
		return found
	case map[string]any:
		var found []string
		for _, item := range typed {
			found = append(found, stringsIn(item)...)
		}
		return found
	}
	return nil
}
