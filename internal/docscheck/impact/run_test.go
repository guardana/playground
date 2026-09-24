package impact_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/guardana/playground/internal/docscheck/impact"
)

const config = `{
  "types": [{"name": "runbook", "heading": "Runbooks", "budget": 1200}],
  "audiences": ["engineering"],
  "readme": {"root": 600, "folder": 300},
  "ceilings": {},
  "directories": {"docs/runbooks": "runbook"},
  "page_types": {},
  "frozen": ["docs/status.md"],
  "excluded": [],
  "surfaces": ["runner/**"]
}`

const runPage = "---\ntitle: Run\nsummary: Run one.\ntype: runbook\naudience: [engineering]\ncovers: [runner/**]\n---\n\n# Run\n"

// fakeGit answers from a table keyed by the joined arguments.
func fakeGit(answers map[string]string) impact.Runner {
	return func(_ context.Context, args ...string) ([]byte, error) {
		out, ok := answers[strings.Join(args, " ")]
		if !ok {
			return nil, errors.New("fatal: not a git repository")
		}
		return []byte(out), nil
	}
}

func run(t *testing.T, git impact.Runner, args ...string) (int, string, string) {
	t.Helper()
	fsys := fstest.MapFS{
		"docs/docs.json":       {Data: []byte(config)},
		"docs/runbooks/run.md": {Data: []byte(runPage)},
		"docs/status.md":       {Data: []byte("# Status\n")},
	}
	seams := impact.Seams{
		FS: fsys,
		Files: func(context.Context) ([]string, error) {
			return []string{"docs/runbooks/run.md", "docs/status.md"}, nil
		},
		Git: git,
	}
	var stdout, stderr bytes.Buffer
	code := impact.Run(context.Background(), args, &stdout, &stderr, seams)
	return code, stdout.String(), stderr.String()
}

func TestARangeNamesThePagesAndTheUnparsedOnes(t *testing.T) {
	git := fakeGit(map[string]string{"diff --name-only --no-renames a..b --": "runner/lab.go\ndocs/status.md\n"})
	code, out, _ := run(t, git, "--range", "a..b")
	for _, want := range []string{"2 changed paths read\n", "  docs/runbooks/run.md <- runner/lab.go\n", "  docs/status.md (docs/status.md)\n", "1 pages did not parse and are missing above\n  docs/status.md: "} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if code != 1 {
		t.Errorf("exit %d with a page that did not parse, want 1", code)
	}
}

func TestWhatGitCannotAnswerIsNotMeasured(t *testing.T) {
	for name, args := range map[string][]string{"a range": {"--range", "a..b"}, "stale": {"--stale"}} {
		code, out, stderr := run(t, fakeGit(nil), args...)
		if code != 2 || !strings.Contains(stderr, "NOT MEASURED") || out != "" {
			t.Errorf("%s: exit %d, stdout %q, stderr %q; want 2 and NOT MEASURED", name, code, out, stderr)
		}
	}
}

func TestAShallowCloneIsNotMeasured(t *testing.T) {
	git := fakeGit(map[string]string{
		"rev-parse --is-shallow-repository": "true\n",
		"log --format=%x00%h %s --name-only --no-renames --diff-merges=first-parent --no-show-signature": "\x00c1 write the page\ndocs/runbooks/run.md\n",
	})
	code, _, stderr := run(t, git, "--stale")
	if code != 2 || !strings.Contains(stderr, "shallow") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

func TestStaleCountsCommitsSinceThePageMoved(t *testing.T) {
	log := "\x00c3 change the runner\nrunner/lab.go\n\n\x00c2 unrelated\nREADME.md\n\n\x00c1 write the page\ndocs/runbooks/run.md\n"
	git := fakeGit(map[string]string{
		"rev-parse --is-shallow-repository": "false\n",
		"log --format=%x00%h %s --name-only --no-renames --diff-merges=first-parent --no-show-signature": log,
	})
	_, out, _ := run(t, git, "--stale")
	want := "1 pages examined\n  docs/runbooks/run.md: last commit c1, 1 commits since touching its covers\n    c3 change the runner\n"
	if !strings.HasPrefix(out, want) {
		t.Errorf("output:\n%s\nwant it to start with\n%s", out, want)
	}
}

func TestForNamesTheCoveringPages(t *testing.T) {
	_, out, _ := run(t, fakeGit(nil), "--for", "runner/lab.go")
	if !strings.HasPrefix(out, "1 pages cover runner/lab.go of 1 examined\n  docs/runbooks/run.md\n") {
		t.Errorf("output:\n%s", out)
	}
}

func TestArgumentsNeedExactlyOneMode(t *testing.T) {
	for _, args := range [][]string{nil, {"--stale", "--for", "x"}, {"--range", "-x"}, {"extra"}} {
		if code, _, _ := run(t, fakeGit(nil), args...); code != 2 {
			t.Errorf("%q: exit %d, want 2", args, code)
		}
	}
}

func TestRepositoryRefusesAnotherRepositorysTopLevel(t *testing.T) {
	dir := t.TempDir()
	git := impact.Repository(fakeGit(map[string]string{"rev-parse --show-toplevel": t.TempDir() + "\n", "diff --name-only --no-renames a..b --": "x\n"}), dir)
	if _, err := impact.Changed(context.Background(), git, "a..b"); !errors.Is(err, impact.ErrNotMeasured) {
		t.Errorf("Changed = %v, want NOT MEASURED", err)
	}
	git = impact.Repository(fakeGit(map[string]string{"rev-parse --show-toplevel": dir + "\n", "diff --name-only --no-renames a..b --": "x\n"}), dir)
	if got, err := impact.Changed(context.Background(), git, "a..b"); err != nil || len(got) != 1 {
		t.Errorf("Changed = %q, %v", got, err)
	}
}

func TestParseLogRefusesAMalformedLog(t *testing.T) {
	for _, out := range []string{"", "runner/lab.go\n", "\x00 no hash\n", "\x00a one\n\x00a again\n", "\x00a one\nbad path\n"} {
		if _, err := impact.ParseLog([]byte(out)); !errors.Is(err, impact.ErrInvalid) {
			t.Errorf("ParseLog(%q) = %v, want a refusal", out, err)
		}
	}
}
