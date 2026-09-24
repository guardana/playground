package docsconfig_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/docscheck/docsconfig"
)

const good = `{
  "types": [
    {"name": "runbook", "heading": "Runbooks", "budget": 1200},
    {"name": "project", "heading": "Project", "exempt": true}
  ],
  "audiences": ["engineering", "product"],
  "readme": {"root": 600, "folder": 300},
  "ceilings": {"victims/shell/README.md": 619},
  "directories": {"docs": "project", "docs/runbooks": "runbook"},
  "page_types": {"docs/runbooks/index.md": "project"},
  "frozen": ["docs/status.md", "victims/*/README.md"],
  "excluded": ["CHANGELOG.md", ".github/"],
  "surfaces": ["runner/**"]
}`

func TestParseReadsTheFile(t *testing.T) {
	c, err := docsconfig.Parse([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if typ, ok := c.Lookup("runbook"); !ok || typ.Budget != 1200 || typ.Heading != "Runbooks" {
		t.Errorf("Lookup(runbook) = %+v, %v", typ, ok)
	}
	for page, want := range map[string]string{
		"docs/status.md": "project", "docs/runbooks/a.md": "runbook", "docs/runbooks/index.md": "project",
	} {
		if got, ok := c.TypeOf(page); !ok || got != want {
			t.Errorf("TypeOf(%s) = %q, %v; want %q", page, got, ok, want)
		}
	}
	if _, ok := c.TypeOf("docs/elsewhere/a.md"); ok {
		t.Error("a page in no listed directory has a type")
	}
	for rel, want := range map[string]bool{
		"CHANGELOG.md": true, ".github/pull_request_template.md": true, "CHANGELOG.md.bak": false, "docs/CHANGELOG.md": false,
	} {
		if got := c.Excludes(rel); got != want {
			t.Errorf("Excludes(%s) = %v, want %v", rel, got, want)
		}
	}
}

func TestParseRefusals(t *testing.T) {
	cases := map[string]struct{ from, to, want string }{
		"an unknown key":             {`"surfaces"`, `"surface": [], "surfaces"`, "unknown field"},
		"a key twice":                {`"audiences"`, `"audiences": ["x"], "audiences"`, "given twice"},
		"no types":                   {`{"name": "runbook", "heading": "Runbooks", "budget": 1200},` + "\n    " + `{"name": "project", "heading": "Project", "exempt": true}`, ``, "types is empty"},
		"a type twice":               {`"name": "project"`, `"name": "runbook"`, "listed twice"},
		"a budget and an exemption":  {`"exempt": true`, `"exempt": true, "budget": 5`, "has a budget and is exempt"},
		"neither budget nor exempt":  {`"budget": 1200`, `"budget": 0`, "no positive budget"},
		"a type without a heading":   {`"heading": "Runbooks"`, `"heading": ""`, "needs a name and a heading"},
		"no audience":                {`["engineering", "product"]`, `[]`, "audiences is empty"},
		"an audience twice":          {`["engineering", "product"]`, `["product", "product"]`, "listed twice"},
		"no readme budget":           {`"root": 600`, `"root": 0`, "readme.root"},
		"a zero ceiling":             {`: 619`, `: 0`, "no positive count"},
		"a directory outside docs":   {`"docs/runbooks": "runbook"`, `"runbooks": "runbook"`, "not a directory under docs"},
		"a directory of no type":     {`"docs/runbooks": "runbook"`, `"docs/runbooks": "tutorial"`, "which types does not declare"},
		"a page type of no type":     {`"docs/runbooks/index.md": "project"`, `"docs/runbooks/index.md": "spec"`, "which types does not declare"},
		"a page type for a non-page": {`"docs/runbooks/index.md"`, `"docs/runbooks"`, "is not a page"},
		"an unclean exclusion":       {`"CHANGELOG.md", `, `"./CHANGELOG.md", `, "not a clean path"},
		"no surfaces":                {`["runner/**"]`, `[]`, "surfaces is empty"},
		"a bad surface glob":         {`["runner/**"]`, `["runner//x"]`, "empty segment"},
		"a bad frozen glob":          {`"docs/status.md", `, `"docs/[x", `, "glob"},
		"text after the document":    {"\n}", "\n} {}", "after the document"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			data := strings.Replace(good, c.from, c.to, 1)
			if data == good {
				t.Fatalf("the edit %q changed nothing", c.from)
			}
			_, err := docsconfig.Parse([]byte(data))
			if !errors.Is(err, docsconfig.ErrInvalid) || !strings.Contains(err.Error(), c.want) {
				t.Errorf("Parse = %v, want a refusal saying %q", err, c.want)
			}
		})
	}
}
