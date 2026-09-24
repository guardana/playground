package main

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// compose/Dockerfile.enforcer builds this directory as a module of its own,
// without the lab's go.mod, so any import outside the standard library breaks
// the enforcer image while every test here stays green.
func TestTheProbeImportsOnlyTheStandardLibrary(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	read := 0
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), source, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		read++
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if first, _, _ := strings.Cut(path, "/"); strings.Contains(first, ".") {
				t.Errorf("%s imports %s, which the enforcer image cannot build", source, path)
			}
		}
	}
	if read == 0 {
		t.Fatal("no source file was read")
	}
}
