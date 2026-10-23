package control

import (
	"os"
	"path/filepath"
	"testing"
)

// A case-insensitive file system gives one directory more than one spelling,
// and resolving links does not make them agree; the key's placement check
// compares directories, not the spelling a person typed.
func TestASecondSpellingOfTheCloneHoldsNoLabKey(t *testing.T) {
	root := filepath.Join(t.TempDir(), "clone")
	if err := os.Mkdir(root, 0o750); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(root), "CLONE")
	if _, err := os.Stat(other); err != nil {
		t.Skip("this file system tells CLONE from clone, so the clone has one spelling here")
	}
	keys := &Driver{config: Config{Root: root, Reports: t.TempDir(), KeysDir: filepath.Join(other, "keys")}}
	if err := keys.refuseKeysPlace(); err == nil {
		t.Error("a lab key under a second spelling of the clone was accepted")
	}
}
