// Package runfile reads files a container could have written into a run
// directory, where a link or an endless file is the container's to plant.
package runfile

import (
	"fmt"
	"io"
	"os"
)

// ReadRegular returns the contents of path when it is a regular file of at
// most limit bytes, never following a link to what it names.
func ReadRegular(path string, limit int) ([]byte, error) {
	named, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !named.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	file, err := os.Open(path) // #nosec G304 -- a regular file, checked again once open.
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(named, opened) {
		return nil, fmt.Errorf("%s changed while it was opened", path)
	}
	body, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err == nil && len(body) > limit {
		err = fmt.Errorf("%s holds more than %d bytes", path, limit)
	}
	return body, err
}
