package labcheck_test

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/docscheck/repofiles"
)

// The verifier scans these files; nothing in the lab loads them. The guards
// below keep them readable as text and unable to run if someone did.
const artifactsDir = "artifacts"

// artifactKinds are the suffixes these guards know how to judge. Another kind
// needs its own guard before it is tracked.
var artifactKinds = []string{".pkl", ".json", ".py", ".yaml", ".md"}

var urlPattern = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>()\x60]+`)

func TestEveryArtifactIsInertText(t *testing.T) {
	problems, files := artifactFileProblems(filepath.Join(repoRoot, artifactsDir))
	if files == 0 {
		t.Fatal("no artifacts found; this guard inspected nothing")
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

func TestEveryArtifactCaseDiffersInExactlyOneFile(t *testing.T) {
	problems, cases := caseProblems(filepath.Join(repoRoot, artifactsDir))
	if cases == 0 {
		t.Fatal("no artifact cases found; this guard inspected nothing")
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// A file or package by the absent module's name would turn every lab pickle's
// failing import into a successful one.
func TestNothingInTheTreeProvidesTheAbsentModule(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	files, err := repofiles.List(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range absentModuleProblems(files) {
		t.Error(problem)
	}
}

func artifactFileProblems(root string) ([]string, int) {
	var problems []string
	files := 0
	err := walkFiles(root, func(fsys fs.FS, name string, entry fs.DirEntry) error {
		files++
		for _, problem := range oneArtifactProblems(fsys, name, entry) {
			problems = append(problems, name+": "+problem)
		}
		return nil
	})
	if err != nil {
		problems = append(problems, err.Error())
	}
	return problems, files
}

func oneArtifactProblems(fsys fs.FS, name string, entry fs.DirEntry) []string {
	if !entry.Type().IsRegular() {
		return []string{"not a regular file"}
	}
	if !slices.Contains(artifactKinds, path.Ext(name)) {
		return []string{"no guard judges a file of this kind"}
	}
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return []string{err.Error()}
	}
	problems := textProblems(data)
	switch {
	case path.Ext(name) == ".pkl":
		if err := checkPickle(data); err != nil {
			problems = append(problems, "pickle: "+err.Error())
		}
	case path.Ext(name) == ".py":
		problems = append(problems, pythonProblems(name, data)...)
	case strings.HasSuffix(name, "config.json"):
		problems = append(problems, hubRepoProblems(data)...)
	}
	return problems
}

// walkFiles visits every entry under dir that is not a directory, by its
// slash path relative to dir, through a root no link can lead out of.
func walkFiles(dir string, visit func(fsys fs.FS, name string, entry fs.DirEntry) error) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	fsys := root.FS()
	return fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		return visit(fsys, name, entry)
	})
}

// textProblems refuses a byte outside printable ASCII, tab and line feed, and
// a URL whose host could resolve.
func textProblems(data []byte) []string {
	var problems []string
	for i, b := range data {
		if (b < 0x20 || b > 0x7e) && b != '\n' && b != '\t' {
			problems = append(problems, fmt.Sprintf("byte 0x%02x at offset %d is not printable ASCII", b, i))
			break
		}
	}
	for _, raw := range urlPattern.FindAllString(string(data), -1) {
		parsed, err := url.Parse(raw)
		if err != nil || !strings.HasSuffix(strings.ToLower(parsed.Hostname()), ".invalid") {
			problems = append(problems, fmt.Sprintf("URL %q names no host under .invalid", raw))
		}
	}
	return problems
}

// caseProblems reads every directory under root as a case holding exactly a
// control and a defect variant with the same file names and one file that
// differs.
func caseProblems(root string) ([]string, int) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return []string{err.Error()}, 0
	}
	var problems []string
	cases := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cases++
		for _, problem := range variantProblems(filepath.Join(root, entry.Name())) {
			problems = append(problems, entry.Name()+": "+problem)
		}
	}
	return problems, cases
}

func variantProblems(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{err.Error()}
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if !slices.Equal(names, []string{"control", "defect"}) {
		return []string{fmt.Sprintf("holds %v, want exactly control and defect", names)}
	}
	control, err := contents(filepath.Join(dir, "control"))
	if err != nil {
		return []string{err.Error()}
	}
	defect, err := contents(filepath.Join(dir, "defect"))
	if err != nil {
		return []string{err.Error()}
	}
	controlNames := slices.Sorted(maps.Keys(control))
	defectNames := slices.Sorted(maps.Keys(defect))
	if !slices.Equal(controlNames, defectNames) {
		return []string{fmt.Sprintf("control holds %v, defect holds %v", controlNames, defectNames)}
	}
	var differ []string
	for _, name := range controlNames {
		if !bytes.Equal(control[name], defect[name]) {
			differ = append(differ, name)
		}
	}
	if len(differ) != 1 {
		return []string{fmt.Sprintf("control and defect differ in %v, want exactly one file", differ)}
	}
	return nil
}

func contents(dir string) (map[string][]byte, error) {
	found := make(map[string][]byte)
	err := walkFiles(dir, func(fsys fs.FS, name string, _ fs.DirEntry) error {
		data, err := fs.ReadFile(fsys, name)
		found[name] = data
		return err
	})
	return found, err
}

// absentModuleProblems refuses any path segment Python could import as the
// absent module: a package directory or a module file of any suffix.
func absentModuleProblems(files []string) []string {
	var problems []string
	for _, file := range files {
		for _, segment := range strings.Split(file, "/") {
			stem, _, _ := strings.Cut(segment, ".")
			if stem == absentModule {
				problems = append(problems, file+" provides "+absentModule)
				break
			}
		}
	}
	return problems
}
