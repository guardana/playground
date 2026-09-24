package pages

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/docscheck/glob"
)

const sourcesPrefix = "Sources: "

// diagramProblems requires every mermaid block to be followed by a Sources
// paragraph whose files the repository tracks and the page's covers match, so
// a change to what a diagram draws makes the page suspect.
func diagramProblems(files []string, covers []glob.Glob, body []byte) []string {
	var problems []string
	lines := strings.Split(string(body), "\n")
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "```mermaid" {
			continue
		}
		start := i + 1
		end := start
		for end < len(lines) && strings.TrimSpace(lines[end]) != "```" {
			end++
		}
		if end == len(lines) {
			return append(problems, fmt.Sprintf("diagram at line %d: no closing fence", start))
		}
		sources, err := readSources(lines[end+1:])
		if err != nil {
			problems = append(problems, fmt.Sprintf("diagram at line %d: %v", start, err))
		}
		for _, src := range sources {
			switch {
			case !slices.Contains(files, src):
				problems = append(problems, fmt.Sprintf("diagram at line %d: source %s is not a file the repository tracks", start, src))
			case !slices.ContainsFunc(covers, func(g glob.Glob) bool { return g.Match(src) }):
				problems = append(problems, fmt.Sprintf("diagram at line %d: source %s is outside the page's covers", start, src))
			}
		}
		i = end
	}
	return problems
}

// readSources reads the paragraph after a fence: `Sources: ` and backticked
// paths separated by a comma and a space, wrapped over lines until a blank
// one, with an optional full stop.
func readSources(after []string) ([]string, error) {
	start := 0
	for start < len(after) && strings.TrimSpace(after[start]) == "" {
		start++
	}
	if start == len(after) || !strings.HasPrefix(after[start], sourcesPrefix) {
		return nil, errors.New("the first line after the fence is not a Sources: line")
	}
	end := start
	for end < len(after) && strings.TrimSpace(after[end]) != "" {
		end++
	}
	text := strings.TrimSuffix(strings.TrimPrefix(strings.Join(after[start:end], " "), sourcesPrefix), ".")
	var sources []string
	for _, item := range strings.Split(text, ", ") {
		src, ok := strings.CutPrefix(item, "`")
		if ok {
			src, ok = strings.CutSuffix(src, "`")
		}
		if !ok || src == "" {
			return nil, fmt.Errorf("the Sources item %q is not one backticked path", item)
		}
		sources = append(sources, src)
	}
	return sources, nil
}
