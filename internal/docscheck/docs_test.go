package docscheck_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/docscheck/repofiles"
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

// A run writes its reports as markdown under reports/, which git ignores. They
// are output, not documentation, and a link in one must never fail the gate.
func TestIgnoredRunReportsAreNotPages(t *testing.T) {
	directory := filepath.Join(repoRoot, "reports")
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		if err := os.Mkdir(directory, 0o750); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := os.Remove(directory); err != nil {
				t.Errorf("planted directory left behind: %v", err)
			}
		}()
	}
	planted := filepath.Join(directory, "planted-docscheck-probe.md")
	if _, err := os.Stat(planted); err == nil {
		t.Fatalf("%s already exists; refusing to overwrite", planted)
	}
	if err := os.WriteFile(planted, []byte("[gone](missing-page.md)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(planted); err != nil {
			t.Errorf("planted file left behind: %v", err)
		}
	}()

	pages := markdownPages(t)
	if slices.Contains(pages, filepath.Join(repoRoot, "reports", "planted-docscheck-probe.md")) {
		t.Errorf("the page walk read %s, which git ignores", planted)
	}
	if !slices.Contains(pages, filepath.Join(repoRoot, "docs", "status.md")) {
		t.Errorf("the page walk missed docs/status.md, so it proves nothing about what it left out")
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

	files, err := repofiles.List(context.Background(), repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	var pages []string
	for _, file := range files {
		if strings.HasSuffix(file, ".md") {
			pages = append(pages, filepath.Join(repoRoot, filepath.FromSlash(file)))
		}
	}
	if len(pages) == 0 {
		t.Fatal("no markdown found; the link check inspected nothing")
	}
	return pages
}
