// Package claims holds what the documentation asserts to the two rules a
// sentence can break: a capability stated without its status, and a count
// stated anywhere but its one home or stated wrong there.
package claims

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// Problem is one line of one file that breaks a rule.
type Problem struct {
	Path   string
	Line   int
	Reason string
}

func (p Problem) String() string { return fmt.Sprintf("%s:%d: %s", p.Path, p.Line, p.Reason) }

// explanations are the pages that tell a reader what the lab does; a
// capability they state reads as a promise as much as the inventory's do.
const explanations = "docs/how-it-works/"

// Unlabelled returns every line of README.md, ROADMAP.md, docs/status.md and
// the pages under docs/how-it-works/ that claims a capability and names no
// status. The first three must be in files: a check that read none of them
// has judged nothing.
func Unlabelled(fsys fs.FS, files []string) ([]Problem, error) {
	held := []string{"README.md", "ROADMAP.md", "docs/status.md"}
	for _, rel := range held {
		if !slices.Contains(files, rel) {
			return nil, fmt.Errorf("claims: %s is not in the file list", rel)
		}
	}
	for _, rel := range files {
		if strings.HasPrefix(rel, explanations) && strings.HasSuffix(rel, ".md") {
			held = append(held, rel)
		}
	}
	var problems []Problem
	for _, rel := range held {
		data, err := fs.ReadFile(fsys, rel)
		if err != nil {
			return nil, err
		}
		for number, line := range strings.Split(string(data), "\n") {
			if isUnlabelledClaim(line) {
				problems = append(problems, Problem{Path: rel, Line: number + 1,
					Reason: fmt.Sprintf("capability claim without a status label: %q", strings.TrimSpace(line))})
			}
		}
	}
	return problems, nil
}

// isUnlabelledClaim reads a line as a capability claim when it holds one of
// the claiming verbs, and as labelled when it names any status.
func isUnlabelledClaim(line string) bool {
	lower := strings.ToLower(line)
	claims := []string{"supports ", "provides ", "enforces ", "integrates with "}
	labels := []string{"planned", "experimental", "implemented", "pre-alpha"}
	return containsAny(lower, claims) && !containsAny(lower, labels)
}

func containsAny(text string, needles []string) bool {
	return slices.ContainsFunc(needles, func(n string) bool { return strings.Contains(text, n) })
}
