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

	"github.com/guardana/playground/internal/docscheck/claims"
	"github.com/guardana/playground/internal/docscheck/markdown"
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

// A link to a heading breaks silently when the heading is renamed: the page
// still opens, at its top.
func TestLinkedHeadingsExist(t *testing.T) {
	problems, err := markdown.BrokenFragments(os.DirFS(repoRoot), listed(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// A capability written without a status label reads as a promise, and promises
// rot silently. Saying "planned" costs one word and stays true.
func TestCapabilityClaimsCarryAStatus(t *testing.T) {
	problems, err := claims.Unlabelled(os.DirFS(repoRoot), listed(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// A count stated on two pages drifts on one of them; it is stated where the
// inventory lives and must be the catalogue a clone holds.
func TestTheScenarioCountHasOneHome(t *testing.T) {
	tracked, err := repofiles.Tracked(context.Background(), repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	problems, err := claims.ScenarioCount(os.DirFS(repoRoot), listed(t), tracked)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
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

func listed(t *testing.T) []string {
	t.Helper()

	files, err := repofiles.List(context.Background(), repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func markdownPages(t *testing.T) []string {
	t.Helper()

	var pages []string
	for _, file := range listed(t) {
		if strings.HasSuffix(file, ".md") {
			pages = append(pages, filepath.Join(repoRoot, filepath.FromSlash(file)))
		}
	}
	if len(pages) == 0 {
		t.Fatal("no markdown found; the link check inspected nothing")
	}
	return pages
}
