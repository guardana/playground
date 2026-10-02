package markdown_test

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/guardana/playground/internal/docscheck/markdown"
)

func TestSlugFollowsGitHub(t *testing.T) {
	for text, want := range map[string]string{
		"A workspace outside the clone":   "a-workspace-outside-the-clone",
		"Verifier scenarios":              "verifier-scenarios",
		"`schema_version` and the `id`":   "schema_version-and-the-id",
		"What it does — and why?":         "what-it-does--and-why",
		"Steps: wait, retry (and resume)": "steps-wait-retry-and-resume",
		"[Linked](other.md) text":         "linked-text",
		"C++ & Go":                        "c--go",
		"Ünïcode Title":                   "ünïcode-title",
		"P0–P3 — Lab":                     "p0p3--lab",
	} {
		if got := markdown.Slug(text); got != want {
			t.Errorf("Slug(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestAnchorsSkipFencesAndNumberRepeats(t *testing.T) {
	body := "---\ntitle: T\n---\n\n# Title\n\n## Run ##\n\n```sh\n# not a heading\n```\n\n" +
		"~~~~\n## nor this\n~~~\n```\n~~~~\n\n## Run\n\n#no space\n    # indented code\n### Run\n"
	want := []string{"title", "run", "run-1", "run-2"}
	if got := markdown.Anchors([]byte(body)); !slices.Equal(got, want) {
		t.Errorf("Anchors = %q, want %q", got, want)
	}
}

func TestFences(t *testing.T) {
	for line, want := range map[string]markdown.Fence{
		"```mermaid":           {Char: '`', Len: 3, Info: "mermaid"},
		"  ~~~~ mermaid title": {Char: '~', Len: 4, Info: "mermaid title"},
		"````":                 {Char: '`', Len: 4},
	} {
		if got, ok := markdown.OpenFence(line); !ok || got != want {
			t.Errorf("OpenFence(%q) = %+v, %v; want %+v", line, got, ok, want)
		}
	}
	for _, line := range []string{"``", "```js `x`", "text ```", "~~"} {
		if got, ok := markdown.OpenFence(line); ok {
			t.Errorf("OpenFence(%q) = %+v, want no fence", line, got)
		}
	}
	backticks, _ := markdown.OpenFence("````")
	tildes, _ := markdown.OpenFence("~~~")
	for _, c := range []struct {
		fence markdown.Fence
		line  string
		want  bool
	}{
		{backticks, "````", true},
		{backticks, "`````", true},
		{backticks, "```", false},
		{backticks, "~~~~", false},
		{backticks, "```` x", false},
		{tildes, " ~~~ ", true},
		{tildes, "```", false},
	} {
		if got := c.fence.Closes(c.line); got != c.want {
			t.Errorf("%+v.Closes(%q) = %v, want %v", c.fence, c.line, got, c.want)
		}
	}
}

func TestBrokenFragmentsNameEachDeadAnchor(t *testing.T) {
	fsys := fstest.MapFS{
		"docs/a.md": {Data: []byte("# A\n\n## Local\n\n" +
			"[ok](b.md#a-real-heading) and [bad](b.md#gone).\n" +
			"[self](#local), [self bad](#nowhere), [repeat](b.md#twice-1).\n" +
			"[web](https://example.test/b.md#x) [code](../runner/lab.go#L10) [absent](c.md#x)\n" +
			"```md\n[fenced](#fenced)\n```\n" +
			"[ref ok]: b.md#twice\n[ref bad]: b.md#gone-too \"Title\"\n" +
			"  [ref self]: <#nowhere-either>\n[ref web]: https://example.test/b.md#x\n" +
			"```\n[ref fenced]: #fenced\n```\n")},
		"docs/b.md":          {Data: []byte("# B\n\n## A real heading\n\n## Twice\n\n## Twice\n")},
		"docs/runbooks/r.md": {Data: []byte("# R\n\n[up](../a.md#local) [up bad](../a.md#Local \"title\")\n")},
		"runner/lab.go":      {Data: []byte("package runner\n")},
	}
	var files []string
	for name := range fsys {
		files = append(files, name)
	}
	problems, err := markdown.BrokenFragments(fsys, files)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range problems {
		got = append(got, p.String())
	}
	slices.Sort(got)
	want := []string{
		"docs/a.md:12: links to b.md#gone-too; docs/b.md has no heading with that anchor",
		"docs/a.md:13: links to #nowhere-either; docs/a.md has no heading with that anchor",
		"docs/a.md:5: links to b.md#gone; docs/b.md has no heading with that anchor",
		"docs/a.md:6: links to #nowhere; docs/a.md has no heading with that anchor",
		"docs/runbooks/r.md:3: links to ../a.md#Local; docs/a.md has no heading with that anchor",
	}
	if !slices.Equal(got, want) {
		t.Errorf("problems:\n%q\nwant:\n%q", got, want)
	}
}
