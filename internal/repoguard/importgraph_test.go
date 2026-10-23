package repoguard_test

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// neutralPackages are the shared formats and the runner's role-neutral core,
// as module-relative path prefixes. None of them may depend on a driver: a
// second tool is graded through them, so they cannot know the first.
func neutralPackages() []string {
	return []string{"internal", "runner/check", "runner/report", "runner/driver", "runner/dockercompose"}
}

// apartDrivers are drivers that never depend on one another, each named with
// the driver it keeps apart from.
func apartDrivers() map[string]string {
	return map[string]string{"drivers/control": "drivers/guardana", "drivers/guardana": "drivers/control"}
}

// importGraph maps each of the module's packages, by module-relative path, to
// every module package it depends on. A test's own imports are not counted:
// a test may build its fixture with a driver's package.
type importGraph map[string][]string

func TestNoNeutralPackageAndNoDriverDependsOnAnotherDriver(t *testing.T) {
	module := modulePath(t)
	graph := readImportGraph(t, module)
	t.Run("the module", func(t *testing.T) {
		for _, crossing := range importCrossings(graph) {
			t.Error(crossing)
		}
	})
	t.Run("planted crossings", func(t *testing.T) {
		planted := importGraph{
			"internal/record":            {"drivers/control/evidence"},
			"runner/driver":              {"drivers/guardana/report"},
			"drivers/control":            {"drivers/guardana/trace"},
			"drivers/guardana/report":    {"drivers/control/answer"},
			"drivers/control/trailcheck": {"runner/check"},
		}
		if found := importCrossings(planted); len(found) != 4 {
			t.Errorf("the planted crossings were reported as %q; want the four that cross", found)
		}
	})
}

// importCrossings names every dependency the layering forbids.
func importCrossings(graph importGraph) []string {
	var found []string
	for pkg, deps := range graph {
		for _, dep := range deps {
			if forbidden(pkg, dep) {
				found = append(found, pkg+" depends on "+dep)
			}
		}
	}
	return found
}

func forbidden(pkg, dep string) bool {
	if under(dep, "drivers") {
		for _, neutral := range neutralPackages() {
			if under(pkg, neutral) {
				return true
			}
		}
	}
	for driver, apart := range apartDrivers() {
		if under(pkg, driver) && under(dep, apart) {
			return true
		}
	}
	return false
}

// under reports whether pkg is dir or a package below it.
func under(pkg, dir string) bool { return pkg == dir || strings.HasPrefix(pkg, dir+"/") }

// readImportGraph asks the go command for each package's dependencies and
// keeps those inside the module.
func readImportGraph(t *testing.T, module string) importGraph {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	format := `{{.ImportPath}}{{range .Deps}} {{.}}{{end}}`
	list := exec.CommandContext(ctx, "go", "list", "-deps", "-f", format, "./...")
	list.Dir = repoRoot
	out, err := list.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	graph := importGraph{}
	lines := bufio.NewScanner(bytes.NewReader(out))
	lines.Buffer(make([]byte, 0, 1<<16), 1<<22)
	for lines.Scan() {
		fields := strings.Fields(lines.Text())
		pkg, inModule := strings.CutPrefix(fields[0], module+"/")
		if !inModule {
			continue
		}
		graph[pkg] = append(graph[pkg], []string{}...)
		for _, dep := range fields[1:] {
			if rel, ok := strings.CutPrefix(dep, module+"/"); ok {
				graph[pkg] = append(graph[pkg], rel)
			}
		}
	}
	if err := lines.Err(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"internal/record", "runner/driver", "drivers/control", "drivers/guardana/report"} {
		if _, found := graph[want]; !found {
			t.Fatalf("go list named no %s; this test would check nothing", want)
		}
	}
	return graph
}
