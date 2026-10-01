// Package runfile reads files a container could have written into a run
// directory, where a link or an endless file is the container's to plant.
package runfile

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

// ReadRegular returns the contents of path when it is a regular file of at
// most limit bytes, never following a link to what it names.
func ReadRegular(path string, limit int) ([]byte, error) {
	return readRegular(path, limit, nil)
}

// readRegular takes afterCheck, run between the check and the open, so a test
// can swap the file in that window.
func readRegular(path string, limit int, afterCheck func()) ([]byte, error) {
	named, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !named.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if afterCheck != nil {
		afterCheck()
	}
	// The file can be replaced after the check: a link swapped in is not
	// followed, and a FIFO is opened without waiting for a writer.
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0) // #nosec G304 -- checked again once open.
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
