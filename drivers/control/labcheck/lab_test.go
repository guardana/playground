// Package labcheck holds no code. It is the guard that the enforcer's own
// configuration in this repository, the gateway parts, the classification and
// the fingerprints, agrees with the scenarios and the victims, read through the
// types the enforcer's driver writes it with.
package labcheck_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const repoRoot = "../../.."

func filesUnder(t *testing.T, directory string) []string {
	t.Helper()
	var found []string
	root := filepath.Join(repoRoot, directory)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".yaml") {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func relative(path string) string {
	trimmed, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return path
	}
	return trimmed
}
