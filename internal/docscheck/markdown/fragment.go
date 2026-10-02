package markdown

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
)

// Problem is one link whose fragment names no heading of its page.
type Problem struct {
	Path   string
	Line   int
	Reason string
}

func (p Problem) String() string { return fmt.Sprintf("%s:%d: %s", p.Path, p.Line, p.Reason) }

var (
	fragmentLink = regexp.MustCompile(`\]\(([^)\s#]*)#([^)\s]+)(?:\s+"[^"]*")?\)`)
	// A label opening with ^ is a footnote, whose text is no destination.
	fragmentDefinition = regexp.MustCompile(`^ {0,3}\[[^\]^][^\]]*\]:[ \t]*<?([^\s<>#]*)#([^\s<>]+)>?` +
		`(?:[ \t]+(?:"[^"]*"|'[^']*'|\([^)]*\)))?[ \t]*$`)
)

// BrokenFragments holds every link from a markdown page in files to a heading
// of a markdown page in files, the same one included, to a heading that page
// has. A link to a page outside files is the link check's to report.
func BrokenFragments(fsys fs.FS, files []string) ([]Problem, error) {
	anchors := map[string][]string{}
	read := func(rel string) ([]string, error) {
		if a, ok := anchors[rel]; ok {
			return a, nil
		}
		data, err := fs.ReadFile(fsys, rel)
		if err != nil {
			return nil, err
		}
		anchors[rel] = Anchors(data)
		return anchors[rel], nil
	}
	var problems []Problem
	for _, rel := range files {
		if !strings.HasSuffix(rel, ".md") {
			continue
		}
		data, err := fs.ReadFile(fsys, rel)
		if err != nil {
			return nil, err
		}
		for _, l := range fragmentLinks(rel, data) {
			if !slices.Contains(files, l.page) {
				continue
			}
			have, err := read(l.page)
			if err != nil {
				return nil, err
			}
			if !slices.Contains(have, l.fragment) {
				problems = append(problems, Problem{Path: rel, Line: l.line,
					Reason: fmt.Sprintf("links to %s; %s has no heading with that anchor", l.text, l.page)})
			}
		}
	}
	return problems, nil
}

type link struct {
	line                 int
	text, page, fragment string
}

// fragmentLinks returns the inline links and link reference definitions of a
// page, outside fenced blocks, that name a fragment of itself or of another
// markdown page in the repository.
func fragmentLinks(rel string, body []byte) []link {
	var links []link
	outsideFences(body, func(number int, line string) {
		matches := fragmentLink.FindAllStringSubmatch(line, -1)
		if m := fragmentDefinition.FindStringSubmatch(line); m != nil {
			matches = append(matches, m)
		}
		for _, m := range matches {
			target := m[1]
			page := rel
			switch {
			case target == "":
			case strings.Contains(target, ":") || !strings.HasSuffix(target, ".md"):
				continue
			case strings.HasPrefix(target, "/"):
				page = strings.TrimPrefix(target, "/")
			default:
				page = path.Join(path.Dir(rel), target)
			}
			links = append(links, link{line: number, text: target + "#" + m[2], page: page, fragment: m[2]})
		}
	})
	return links
}
