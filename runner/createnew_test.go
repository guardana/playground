package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/runner/report"
)

// A service can plant a link wherever it can write, so the runner never writes
// through one: the report is refused and the link's target keeps its bytes.
func TestALinkPlantedAtTheReportIsRefusedAndItsTargetUntouched(t *testing.T) {
	runDir, outside := t.TempDir(), t.TempDir()
	target := filepath.Join(outside, "host-file")
	if err := os.WriteFile(target, []byte("the host's own bytes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(runDir, "report.md")); err != nil {
		t.Fatal(err)
	}
	err := writeReports(runDir, assertion.Report{Scenario: "s", RunID: "r"}, nil, report.Provenance{})
	if err == nil {
		t.Fatal("the report was written through a link")
	}
	body, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(body) != "the host's own bytes\n" {
		t.Errorf("the link's target now holds %q", body)
	}
}

func TestAFileAlreadyAtAPathIsNeitherTruncatedNorReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boot.json")
	if err := os.WriteFile(path, []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(path, map[string]string{"second": "x"}); err == nil {
		t.Error("a second write to one path was taken")
	}
	if err := writeBytes(path, []byte("third\n")); err == nil {
		t.Error("a second write to one path was taken")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "first\n" {
		t.Errorf("the file now holds %q", body)
	}
}

// Every write the runner makes goes through the helper that refuses an existing
// path; a write site that truncates or replaces would follow a planted link.
func TestNoRunnerSourceWritesThroughAnExistingPath(t *testing.T) {
	sources := runnerSources(t)
	if len(sources) < 10 {
		t.Fatalf("scanned %d source files, want this package's own", len(sources))
	}
	for _, source := range sources {
		body, err := os.ReadFile(source) // #nosec G304 -- this package's own sources.
		if err != nil {
			t.Fatal(err)
		}
		problems, err := writeSites(source, body)
		if err != nil {
			t.Fatal(err)
		}
		for _, problem := range problems {
			t.Error(problem)
		}
	}
}

// runnerSources lists this package's sources and its subpackages', tests aside.
func runnerSources(t *testing.T) []string {
	t.Helper()
	var sources []string
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			sources = append(sources, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return sources
}

// writeSites names each place in a source that could write through a path
// that already exists.
func writeSites(source string, body []byte) ([]string, error) {
	var problems []string
	for _, needle := range []string{"os." + "WriteFile(", "O_" + "TRUNC", "os." + "Create("} {
		if strings.Contains(string(body), needle) {
			problems = append(problems, source+" uses "+needle+"; write through createNew instead")
		}
	}
	opens, err := opensForWriting(source, body)
	for _, at := range opens {
		problems = append(problems, at+" opens a file for writing without O_CREATE|O_EXCL; write through createNew instead")
	}
	return problems, err
}

func TestTheWriteGuardReadsEveryOpenForWriting(t *testing.T) {
	for sample, want := range map[string]int{
		"os.OpenFile(p, os.O_WRONLY|os.O_CREATE, 0o600)":           1,
		"os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0o600)":           1,
		"os.OpenFile(p,\n\tos.O_RDWR|os.O_EXCL,\n\t0o600)":         1,
		"os.OpenFile(p, flags, 0o600)":                             1,
		"os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)": 0,
		"os.OpenFile(p, os.O_RDONLY, 0)":                           0,
	} {
		source := "package p\n\nimport \"os\"\n\nfunc f(p string, flags int) {\n\t_, _ = " + sample + "\n}\n"
		opens, err := opensForWriting("sample.go", []byte(source))
		if err != nil {
			t.Fatal(err)
		}
		if len(opens) != want {
			t.Errorf("%s: the guard named %v, want %d", sample, opens, want)
		}
	}
}

// opensForWriting names each os.OpenFile call in a source whose flags write
// without both O_CREATE and O_EXCL, or cannot be read as os.O_* constants.
func opensForWriting(name string, source []byte) ([]string, error) {
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, name, source, 0)
	if err != nil {
		return nil, err
	}
	var found []string
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 || !isOSCall(call.Fun, "OpenFile") {
			return true
		}
		if !excludesExisting(osConstants(call.Args[1])) {
			found = append(found, files.Position(call.Pos()).String())
		}
		return true
	})
	return found, nil
}

// excludesExisting holds for flags that only read, or that create a new file
// and refuse an existing one; flags not written as os.O_* constants are judged
// as neither.
func excludesExisting(flags map[string]bool) bool {
	if len(flags) == 0 {
		return false
	}
	writes := flags["O_WRONLY"] || flags["O_RDWR"] || flags["O_APPEND"] || flags["O_CREATE"] || flags["O_TRUNC"]
	return !writes || flags["O_CREATE"] && flags["O_EXCL"]
}

func isOSCall(fun ast.Expr, name string) bool {
	selector, ok := fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "os"
}

func osConstants(expr ast.Expr) map[string]bool {
	names := map[string]bool{}
	ast.Inspect(expr, func(node ast.Node) bool {
		if selector, ok := node.(*ast.SelectorExpr); ok && strings.HasPrefix(selector.Sel.Name, "O_") {
			names[selector.Sel.Name] = true
		}
		return true
	})
	return names
}
