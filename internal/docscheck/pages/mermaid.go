package pages

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/docscheck/glob"
	"github.com/guardana/playground/internal/docscheck/markdown"
)

const sourcesPrefix = "Sources: "

// diagramProblems requires every mermaid block, opened by backticks or tildes
// with an info string starting with mermaid, to name a known diagram type on
// its first line and to be followed by a Sources paragraph whose files the
// repository tracks and the page's covers match, so a change to what a
// diagram draws makes the page suspect.
func diagramProblems(files []string, covers []glob.Glob, body []byte) []string {
	var problems []string
	lines := strings.Split(string(body), "\n")
	for i := 0; i < len(lines); i++ {
		fence, ok := markdown.OpenFence(lines[i])
		if !ok {
			continue
		}
		start := i + 1
		end := start
		for end < len(lines) && !fence.Closes(lines[end]) {
			end++
		}
		if end == len(lines) {
			return append(problems, fmt.Sprintf("block at line %d: no closing fence", start))
		}
		i = end
		if !strings.HasPrefix(fence.Info, "mermaid") {
			continue
		}
		for _, p := range diagramTypeProblem(lines[start:end]) {
			problems = append(problems, fmt.Sprintf("diagram at line %d: %s", start, p))
		}
		for _, p := range sourceProblems(files, covers, lines[end+1:]) {
			problems = append(problems, fmt.Sprintf("diagram at line %d: %s", start, p))
		}
	}
	return problems
}

// diagramTypeProblem refuses a block whose first line is not one of the
// diagram types the documentation draws with.
func diagramTypeProblem(block []string) []string {
	for _, line := range block {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "flowchart", "sequenceDiagram", "stateDiagram-v2", "erDiagram":
			return nil
		}
		return []string{fmt.Sprintf("the first line %q names no diagram type of flowchart, sequenceDiagram, stateDiagram-v2, erDiagram", strings.TrimSpace(line))}
	}
	return []string{"the block names no diagram type: it is empty"}
}

func sourceProblems(files []string, covers []glob.Glob, after []string) []string {
	sources, err := readSources(after)
	if err != nil {
		return []string{err.Error()}
	}
	var problems []string
	for _, src := range sources {
		switch {
		case !slices.Contains(files, src):
			problems = append(problems, fmt.Sprintf("source %s is not a file the repository tracks", src))
		case !slices.ContainsFunc(covers, func(g glob.Glob) bool { return g.Match(src) }):
			problems = append(problems, fmt.Sprintf("source %s is outside the page's covers", src))
		}
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
