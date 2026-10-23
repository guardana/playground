package repoguard_test

import (
	"bufio"
	"bytes"
	"go/build"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/guardana/playground/internal/docscheck/glob"
)

type depguardRule struct {
	Files []string `json:"files"`
	Deny  []struct {
		Pkg string `json:"pkg"`
	} `json:"deny"`
}

type lintConfig struct {
	Linters struct {
		Settings struct {
			Depguard struct {
				Rules map[string]depguardRule `json:"rules"`
			} `json:"depguard"`
		} `json:"settings"`
	} `json:"linters"`
}

// moduleTree is what a depguard rule can name: the module's Go files and the
// directories that hold them, as slash paths relative to the module root.
type moduleTree struct {
	module   string
	files    []string
	packages map[string]bool
}

// A rule whose files glob matches no Go file, or whose deny path names no
// package, guards nothing; it passes lint on every tree, including one that
// moved the package it was written for.
func TestEveryDepguardRuleNamesSomethingInTheModule(t *testing.T) {
	tree := readModuleTree(t)
	t.Run("the configuration", func(t *testing.T) {
		rules := readDepguardRules(t)
		for _, miss := range depguardMisses(t, rules, tree) {
			t.Error(miss)
		}
	})
	t.Run("a glob and a deny path that match nothing", func(t *testing.T) {
		var planted depguardRule
		planted.Files = []string{"**/internal/" + "nowhere/**"}
		planted.Deny = append(planted.Deny, struct {
			Pkg string `json:"pkg"`
		}{Pkg: tree.module + "/nowhere"})
		if misses := depguardMisses(t, map[string]depguardRule{"planted": planted}, tree); len(misses) != 2 {
			t.Fatalf("the planted rule was reported as %q; want the glob and the deny path", misses)
		}
	})
}

func readDepguardRules(t *testing.T) map[string]depguardRule {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg lintConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf(".golangci.yml: %v", err)
	}
	rules := cfg.Linters.Settings.Depguard.Rules
	if len(rules) == 0 {
		t.Fatal(".golangci.yml declares no depguard rule; this test would check nothing")
	}
	return rules
}

func depguardMisses(t *testing.T, rules map[string]depguardRule, tree moduleTree) []string {
	t.Helper()
	var misses []string
	for name, rule := range rules {
		if len(rule.Files) == 0 || len(rule.Deny) == 0 {
			misses = append(misses, "depguard rule "+name+" names no files or denies nothing")
		}
		for _, pattern := range rule.Files {
			if !globMatchesAFile(t, pattern, tree.files) {
				misses = append(misses, "depguard rule "+name+": files glob "+pattern+" matches no Go file")
			}
		}
		for _, deny := range rule.Deny {
			if !denyNamesAPackage(deny.Pkg, tree) {
				misses = append(misses, "depguard rule "+name+": deny path "+deny.Pkg+" names no package")
			}
		}
	}
	return misses
}

// globMatchesAFile reads a depguard files entry. "$all" and "$test" are
// depguard's own keywords, and a leading "!" negates a pattern that must still
// match something to exclude it.
func globMatchesAFile(t *testing.T, pattern string, files []string) bool {
	t.Helper()
	if pattern == "$all" || pattern == "$test" {
		return true
	}
	compiled, err := glob.Compile(strings.TrimPrefix(pattern, "!"))
	if err != nil {
		t.Fatalf("depguard files glob %q: %v", pattern, err)
	}
	for _, file := range files {
		if compiled.Match(file) {
			return true
		}
	}
	return false
}

// denyNamesAPackage follows depguard's prefix match, where a trailing "$"
// asks for the exact path. A path outside the module must be a standard
// library package.
func denyNamesAPackage(pkg string, tree moduleTree) bool {
	exact := strings.HasSuffix(pkg, "$")
	pkg = strings.TrimSuffix(pkg, "$")
	if pkg == tree.module || strings.HasPrefix(pkg, tree.module+"/") {
		rel := strings.TrimPrefix(strings.TrimPrefix(pkg, tree.module), "/")
		for dir := range tree.packages {
			if dir == rel || (!exact && (rel == "" || strings.HasPrefix(dir, rel+"/"))) {
				return true
			}
		}
		return false
	}
	found, err := build.Default.Import(pkg, "", build.FindOnly)
	return err == nil && found.Goroot
}

func readModuleTree(t *testing.T) moduleTree {
	t.Helper()
	tree := moduleTree{module: modulePath(t), packages: map[string]bool{}}
	err := filepath.WalkDir(repoRoot, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && p != repoRoot && strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(p, ".go") {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		tree.files = append(tree.files, rel)
		tree.packages[path.Dir(rel)] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.files) == 0 {
		t.Fatal("the module holds no Go file; this test would check nothing")
	}
	return tree
}

func modulePath(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	lines := bufio.NewScanner(bytes.NewReader(raw))
	for lines.Scan() {
		if name, ok := strings.CutPrefix(lines.Text(), "module "); ok {
			return strings.TrimSpace(name)
		}
	}
	t.Fatal("go.mod names no module")
	return ""
}
