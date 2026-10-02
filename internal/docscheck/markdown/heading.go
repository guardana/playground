package markdown

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Heading is one ATX heading outside a fenced block.
type Heading struct {
	Line  int
	Level int
	Text  string
}

var (
	atxHeading = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$`)
	inlineLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
)

// Headings returns the page's ATX headings in order. Setext headings are not
// read: a frontmatter block's closing line would look like one.
func Headings(body []byte) []Heading {
	var found []Heading
	outsideFences(body, func(number int, line string) {
		m := atxHeading.FindStringSubmatch(line)
		if m == nil {
			return
		}
		found = append(found, Heading{Line: number, Level: len(m[1]), Text: strings.TrimSpace(m[2])})
	})
	return found
}

// Slug is the anchor GitHub gives a heading's text: the rendered text in
// lower case, every space a hyphen, and every character but a letter, a
// digit, a mark, a connector or a hyphen dropped.
func Slug(text string) string {
	text = inlineLink.ReplaceAllString(text, "$1")
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || unicode.Is(unicode.Pc, r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Anchors returns the anchor of every heading of the page; a repeated slug
// takes -1, -2 and so on, as GitHub numbers them.
func Anchors(body []byte) []string {
	seen := map[string]int{}
	var anchors []string
	for _, h := range Headings(body) {
		slug := Slug(h.Text)
		anchor := slug
		if n := seen[slug]; n > 0 {
			anchor = slug + "-" + strconv.Itoa(n)
		}
		seen[slug]++
		anchors = append(anchors, anchor)
	}
	return anchors
}
