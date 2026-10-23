package runfile

import (
	"errors"
	"io/fs"
	"os"
)

// Shared is the mode of a run directory every service writes into: anyone can
// add a record, and nobody but its owner can remove or replace one.
const Shared = fs.ModeSticky | 0o777

// CreateNew is every write into a run directory, whose subdirectories every
// service can write into: a link or file already at path is refused, never
// followed or truncated, so each path is written once.
func CreateNew(path string, mode fs.FileMode, body func(*os.File) error) error {
	// #nosec G304,G703 -- the path is inside the run directory the runner made.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	return errors.Join(body(file), file.Close())
}

// MakeDir sets the mode again after Mkdir, whose mode the umask of whoever ran
// the runner narrows.
func MakeDir(path string, mode fs.FileMode) error {
	if err := os.Mkdir(path, mode); err != nil { // #nosec G301,G703 -- the caller names the mode and why.
		return err
	}
	return os.Chmod(path, mode) // #nosec G302,G703 -- as above.
}
