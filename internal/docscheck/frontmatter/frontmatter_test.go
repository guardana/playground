package frontmatter_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/docscheck/frontmatter"
)

// good is a block every rule accepts; each refusal below makes one edit to it.
const good = "---\n" +
	"title: Run a scenario\n" +
	"summary: Run one scenario or the catalogue and read what the runner graded.\n" +
	"type: runbook\n" +
	"audience: [engineering, product]\n" +
	"covers: [runner/**, scenarios/*.yaml]\n" +
	"---\n"

const body = "\n# Run a scenario\n\nThe steps.\n"

func TestParseReadsABlockAndItsBody(t *testing.T) {
	m, got, err := frontmatter.Parse([]byte(good + body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := frontmatter.Meta{
		Title:    "Run a scenario",
		Summary:  "Run one scenario or the catalogue and read what the runner graded.",
		Type:     "runbook",
		Audience: []string{"engineering", "product"},
		Covers:   []string{"runner/**", "scenarios/*.yaml"},
	}
	if !equal(m, want) {
		t.Errorf("Parse = %+v, want %+v", m, want)
	}
	if string(got) != body {
		t.Errorf("body = %q, want %q", got, body)
	}
}

func TestParseReadsGenerated(t *testing.T) {
	page := strings.TrimSuffix(good, "---\n") + "generated: scripts/gen-docs-index.go\n---\n"
	m, _, err := frontmatter.Parse([]byte(page))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if m.Generated != "scripts/gen-docs-index.go" {
		t.Errorf("Generated = %q", m.Generated)
	}
}

func TestParseRefusals(t *testing.T) {
	long := strings.Repeat("s", frontmatter.MaxSummary+1)
	many := make([]string, frontmatter.MaxCovers+1)
	for i := range many {
		many[i] = "runner/x" + strings.Repeat("y", i) + "/**"
	}
	edit := func(from, to string) string { return strings.Replace(good, from, to, 1) }
	cases := map[string]struct{ page, want string }{
		"empty page":                         {"", "line 1 is not"},
		"no opening fence":                   {strings.TrimPrefix(good, "---\n"), "line 1 is not"},
		"a carriage return":                  {strings.ReplaceAll(good, "\n", "\r\n"), "carriage return"},
		"no closing fence":                   {strings.TrimSuffix(good, "\n---\n"), "no closing"},
		"no newline after the closing fence": {strings.TrimSuffix(good, "\n"), "no newline after it"},
		"an unknown key":                     {edit("type: runbook\n", "type: runbook\nauthor: me\n"), "unknown key"},
		"a key twice":                        {edit("type: runbook\n", "type: runbook\ntype: runbook\n"), "given twice"},
		"keys out of order":                  {edit("type: runbook\naudience: [engineering, product]\n", "audience: [engineering]\ntype: runbook\n"), "comes before"},
		"no space after the colon":           {edit("type: runbook", "type:runbook"), "not a key, a colon"},
		"no title":                           {edit("title: Run a scenario\n", ""), "title is required"},
		"no summary":                         {edit("summary: Run one scenario or the catalogue and read what the runner graded.\n", ""), "summary is required"},
		"a long summary":                     {edit("summary: Run one scenario or the catalogue and read what the runner graded.", "summary: "+long), "summary is 161"},
		"no type":                            {edit("type: runbook\n", ""), "type is required"},
		"no audience":                        {edit("audience: [engineering, product]\n", ""), "audience is required"},
		"an empty audience":                  {edit("audience: [engineering, product]", "audience: []"), "audience is required"},
		"an audience twice":                  {edit("audience: [engineering, product]", "audience: [product, product]"), "twice"},
		"no covers":                          {edit("covers: [runner/**, scenarios/*.yaml]\n", ""), "covers is required"},
		"an empty covers":                    {edit("covers: [runner/**, scenarios/*.yaml]", "covers: []"), "covers is required"},
		"too many covers":                    {edit("covers: [runner/**, scenarios/*.yaml]", "covers: ["+strings.Join(many, ", ")+"]"), "covers lists 33"},
		"a glob twice":                       {edit("covers: [runner/**, scenarios/*.yaml]", "covers: [runner/**, runner/**]"), "twice"},
		"a list without brackets":            {edit("covers: [runner/**, scenarios/*.yaml]", "covers: runner/**"), "starts with ["},
		"a list with two spaces":             {edit("[runner/**, scenarios/*.yaml]", "[runner/**,  scenarios/*.yaml]"), "white space"},
		"a quoted title":                     {edit("title: Run a scenario", "title: \"Run a scenario\""), "YAML reads as syntax"},
		"a title YAML reads as a boolean":    {edit("title: Run a scenario", "title: yes"), "boolean"},
		"a title YAML reads as a number":     {edit("title: Run a scenario", "title: 1.5"), "number"},
		"a comment in a value":               {edit("title: Run a scenario", "title: Run #1"), "comment"},
		"a generator outside scripts":        {strings.TrimSuffix(good, "---\n") + "generated: tools/gen-x.go\n---\n", "does not name"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			m, rest, err := frontmatter.Parse([]byte(c.page))
			if !errors.Is(err, frontmatter.ErrInvalid) {
				t.Fatalf("Parse accepted the page: %+v", m)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not say %q", err, c.want)
			}
			if rest != nil {
				t.Errorf("a refused page handed back a body")
			}
		})
	}
}

func TestRenderIsWhatParseReads(t *testing.T) {
	m, _, err := frontmatter.Parse([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	m.Generated = "scripts/gen-docs-index.go"
	block, err := frontmatter.Render(m)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSuffix(good, "---\n") + "generated: scripts/gen-docs-index.go\n---\n"
	if string(block) != want {
		t.Errorf("Render =\n%s\nwant\n%s", block, want)
	}
	back, _, err := frontmatter.Parse(append(block, body...))
	if err != nil || !equal(back, m) {
		t.Errorf("Parse(Render(m)) = %+v, %v; want %+v", back, err, m)
	}
}

func TestRenderRefusesWhatValidateRefuses(t *testing.T) {
	if _, err := frontmatter.Render(frontmatter.Meta{Title: "A", Summary: "B", Type: "runbook"}); !errors.Is(err, frontmatter.ErrInvalid) {
		t.Errorf("Render wrote a block with no audience and no covers: %v", err)
	}
}

func TestWordsCountsProseOutsideFences(t *testing.T) {
	page := "# Title here\n\n| a | b |\n|---|---|\n\n```\nnot counted at all\n```\n\n- one, two\n"
	if got := frontmatter.Words([]byte(page)); got != 6 {
		t.Errorf("Words = %d, want 6", got)
	}
}

func TestHasBlock(t *testing.T) {
	if !frontmatter.HasBlock([]byte(good)) || frontmatter.HasBlock([]byte("# Title\n")) {
		t.Error("HasBlock does not tell a block from a heading")
	}
}

func equal(a, b frontmatter.Meta) bool {
	return a.Title == b.Title && a.Summary == b.Summary && a.Type == b.Type &&
		slices.Equal(a.Audience, b.Audience) && slices.Equal(a.Covers, b.Covers) && a.Generated == b.Generated
}
