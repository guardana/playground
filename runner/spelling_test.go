package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A case-insensitive file system gives one directory more than one spelling,
// and resolving links does not make them agree; the placement checks compare
// directories, not the spelling a person typed.
func TestASecondSpellingOfTheCloneIsTheClone(t *testing.T) {
	root := filepath.Join(t.TempDir(), "clone")
	for _, dir := range []string{root, filepath.Join(root, "trajectories")} {
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	other := filepath.Join(filepath.Dir(root), "CLONE")
	if _, err := os.Stat(other); err != nil {
		t.Skip("this file system tells CLONE from clone, so the clone has one spelling here")
	}
	if _, err := openWorkspace(root, filepath.Join(other, "trajectories", "runs"), nil); err == nil {
		t.Error("reports under a second spelling of trajectories/ were accepted")
	}
	keys := lab{root: root, reports: t.TempDir(), keysDir: filepath.Join(other, "keys")}
	if err := keys.refuseKeysPlace(); err == nil {
		t.Error("a lab key under a second spelling of the clone was accepted")
	}
}
