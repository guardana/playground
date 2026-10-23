package control

import (
	"fmt"
	"os"
	"time"

	"github.com/guardana/playground/internal/runfile"
)

// What the compose topology names for the enforcer and the services around it.
const (
	servicePort = "8080"
	// publicDirMode keeps a directory the runner writes readable by the
	// service that mounts it.
	publicDirMode = 0o755
)

func (d *Driver) note(format string, values ...any) {
	if d.config.Log == nil {
		return
	}
	_, _ = fmt.Fprintf(d.config.Log, "runner: "+format+"\n", values...)
}

func (d *Driver) clock() time.Time {
	if d.config.Clock == nil {
		return time.Now()
	}
	return d.config.Clock()
}

func writeBytes(path string, body []byte) error {
	return write(path, func(file *os.File) error { _, err := file.Write(body); return err })
}

func write(path string, body func(*os.File) error) error { return runfile.CreateNew(path, 0o600, body) }

// makeShared creates one directory every service can write into.
func makeShared(path string) error { return runfile.MakeDir(path, runfile.Shared) }

func or(value, otherwise string) string {
	if value == "" {
		return otherwise
	}
	return value
}
