package repoguard_test

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// neutralPackages are the shared formats and the runner's role-neutral core,
// as module-relative path prefixes. None of them may depend on a driver, not
// even in a test: a second tool is graded through them, so they cannot know
// the first.
func neutralPackages() []string {
	return []string{"internal", "runner/check", "runner/report", "runner/driver", "runner/dockercompose"}
}

// importGraph maps each of the module's packages, by module-relative path, to
// every module package it or its tests depend on.
type importGraph map[string][]string

func TestNoNeutralPackageAndNoDriverDependsOnAnotherDriver(t *testing.T) {
	module := modulePath(t)
	drivers := readDrivers(t)
	graph := readImportGraph(t, module, drivers)
	t.Run("the module", func(t *testing.T) {
		for _, crossing := range importCrossings(graph, drivers) {
			t.Error(crossing)
		}
	})
	t.Run("planted crossings", func(t *testing.T) {
		planted := importGraph{
			"internal/record":          {"drivers/alpha/evidence"},
			"runner/driver":            {"drivers/beta/report"},
			"drivers/alpha":            {"drivers/beta/trace"},
			"drivers/beta/report":      {"drivers/alpha/answer"},
			"drivers/alpha/trailcheck": {"runner/check"},
		}
		if found := importCrossings(planted, []string{"drivers/alpha", "drivers/beta"}); len(found) != 4 {
			t.Errorf("the planted crossings were reported as %q; want the four that cross", found)
		}
	})
	t.Run("a crossing only a test makes", func(t *testing.T) {
		const m = "example.com/lab"
		listed := m + "/internal/x_test [" + m + "/internal/x.test]\t" + m + "/internal/x [" + m + "/internal/x.test]\t" +
			m + "/drivers/alpha/gateway\n" + m + "/internal/x.test\t" + m + "/internal/x_test [" + m + "/internal/x.test]\n"
		graph, err := parseImportGraph([]byte(listed), m)
		if err != nil {
			t.Fatal(err)
		}
		if found := importCrossings(graph, []string{"drivers/alpha"}); len(found) != 1 || found[0] != "internal/x depends on drivers/alpha/gateway" {
			t.Errorf("a test's import of a driver was reported as %q", found)
		}
	})
}

// importCrossings names every dependency the layering forbids.
func importCrossings(graph importGraph, drivers []string) []string {
	var found []string
	for pkg, deps := range graph {
		for _, dep := range deps {
			if forbidden(pkg, dep, drivers) {
				found = append(found, pkg+" depends on "+dep)
			}
		}
	}
	return found
}

// forbidden holds that a neutral package depends on no driver, and that no
// driver depends on another.
func forbidden(pkg, dep string, drivers []string) bool {
	if under(dep, "drivers") {
		for _, neutral := range neutralPackages() {
			if under(pkg, neutral) {
				return true
			}
		}
	}
	for _, driver := range drivers {
		if under(pkg, driver) && under(dep, "drivers") && !under(dep, driver) {
			return true
		}
	}
	return false
}

// under reports whether pkg is dir or a package below it.
func under(pkg, dir string) bool { return pkg == dir || strings.HasPrefix(pkg, dir+"/") }

// readDrivers lists each directory under drivers/ as a module-relative path.
func readDrivers(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot, "drivers"))
	if err != nil {
		t.Fatal(err)
	}
	var drivers []string
	for _, entry := range entries {
		if entry.IsDir() {
			drivers = append(drivers, "drivers/"+entry.Name())
		}
	}
	if len(drivers) == 0 {
		t.Fatal("no driver under drivers/; this test would check nothing")
	}
	return drivers
}

// readImportGraph asks the go command for each package's dependencies, its
// tests' included, and keeps those inside the module.
func readImportGraph(t *testing.T, module string, drivers []string) importGraph {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	format := "{{.ImportPath}}{{range .Deps}}\t{{.}}{{end}}"
	list := exec.CommandContext(ctx, "go", "list", "-deps", "-test", "-f", format, "./...")
	list.Dir = repoRoot
	out, err := list.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	graph, err := parseImportGraph(out, module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range append([]string{"internal/record", "runner/driver"}, drivers...) {
		if _, found := graph[want]; !found {
			t.Fatalf("go list named no %s; this test would check nothing", want)
		}
	}
	return graph
}

// parseImportGraph reads go list's lines, an import path and then its
// dependencies, tab-separated. A test variant ("p [p.test]"), an external test
// package (p_test) and a test binary (p.test) all count as p.
func parseImportGraph(out []byte, module string) (importGraph, error) {
	graph := importGraph{}
	lines := bufio.NewScanner(bytes.NewReader(out))
	lines.Buffer(make([]byte, 0, 1<<16), 1<<22)
	for lines.Scan() {
		fields := strings.Split(lines.Text(), "\t")
		pkg, inModule := strings.CutPrefix(packageOf(fields[0]), module+"/")
		if !inModule {
			continue
		}
		graph[pkg] = append(graph[pkg], []string{}...)
		for _, dep := range fields[1:] {
			if rel, ok := strings.CutPrefix(packageOf(dep), module+"/"); ok && rel != pkg {
				graph[pkg] = append(graph[pkg], rel)
			}
		}
	}
	for pkg, deps := range graph {
		slices.Sort(deps)
		graph[pkg] = slices.Compact(deps)
	}
	return graph, lines.Err()
}

func packageOf(importPath string) string {
	importPath, _, _ = strings.Cut(importPath, " [")
	importPath = strings.TrimSuffix(importPath, ".test")
	return strings.TrimSuffix(importPath, "_test")
}
