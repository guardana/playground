package docscheck_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const repoRoot = "../.."

// AGENTS.md is read before every change. Past a couple of screens it stops
// being read, and a rule nobody reads is not a rule.
func TestProjectRulesStayShort(t *testing.T) {
	const limit = 200

	body, err := os.ReadFile(filepath.Join(repoRoot, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}

	lines := bytes.Count(body, []byte("\n"))
	if lines > limit {
		t.Errorf("AGENTS.md is %d lines; the limit is %d", lines, limit)
	}
}

var markdownLink = regexp.MustCompile(`]\((\.{0,2}/[^)#\s]+|[A-Za-z0-9_./-]+\.md)`)

func TestLocalLinksResolve(t *testing.T) {
	for _, page := range markdownPages(t) {
		body, err := os.ReadFile(page)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range markdownLink.FindAllStringSubmatch(string(body), -1) {
			target := filepath.Join(filepath.Dir(page), match[1])
			if _, err := os.Stat(target); err != nil {
				t.Errorf("%s links to %s, which does not exist", page, match[1])
			}
		}
	}
}

// A capability written without a status label reads as a promise, and promises
// rot silently. Saying "planned" costs one word and stays true.
func TestCapabilityClaimsCarryAStatus(t *testing.T) {
	labels := []string{"planned", "experimental", "implemented", "pre-alpha"}
	claims := []string{"supports ", "provides ", "enforces ", "integrates with "}

	for _, page := range []string{"README.md", "ROADMAP.md", "docs/status.md"} {
		body, err := os.ReadFile(filepath.Join(repoRoot, page))
		if err != nil {
			t.Fatal(err)
		}
		for number, line := range strings.Split(string(body), "\n") {
			lower := strings.ToLower(line)
			if !containsAny(lower, claims) || containsAny(lower, labels) {
				continue
			}
			t.Errorf("%s:%d: capability claim without a status label: %q", page, number+1, strings.TrimSpace(line))
		}
	}
}

func containsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func markdownPages(t *testing.T) []string {
	t.Helper()

	var pages []string
	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path == repoRoot {
				return nil
			}
			if name := entry.Name(); strings.HasPrefix(name, ".") && name != ".github" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".md") {
			pages = append(pages, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 {
		t.Fatal("no markdown found; the link check inspected nothing")
	}
	return pages
}
