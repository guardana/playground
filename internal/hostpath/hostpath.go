// Package hostpath compares paths on the host the way the file system
// resolves them, so a link or a second spelling of a directory does not carry
// one place past a check that keeps two apart.
package hostpath

import (
	"os"
	"path/filepath"
	"strings"
)

// Resolved is path made absolute with every link resolved. A path that does
// not exist yet keeps its missing tail under its nearest existing directory,
// resolved, so it compares with paths that do exist.
func Resolved(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if evaluated, err := filepath.EvalSymlinks(absolute); err == nil {
		return evaluated
	}
	parent := filepath.Dir(absolute)
	if parent == absolute {
		return absolute
	}
	return filepath.Join(Resolved(parent), filepath.Base(absolute))
}

// Within reports whether path is dir or lies below it. Spellings are compared
// first; then every existing ancestor of path is compared with dir as a file,
// because a case-insensitive file system or a second name for a volume gives
// one directory several spellings that resolving links does not reconcile.
func Within(path, dir string) bool {
	relative, err := filepath.Rel(dir, path)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return true
	}
	target, err := os.Stat(dir)
	if err != nil {
		return false
	}
	for at := filepath.Clean(path); ; at = filepath.Dir(at) {
		if info, err := os.Stat(at); err == nil && os.SameFile(info, target) {
			return true
		}
		if filepath.Dir(at) == at {
			return false
		}
	}
}
