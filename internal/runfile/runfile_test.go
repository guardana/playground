package runfile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/runfile"
)

func TestOnlyARegularFileWithinTheBoundIsRead(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	within := write("within", strings.Repeat("x", 8))
	past := write("past", strings.Repeat("x", 9))
	link := filepath.Join(dir, "link")
	if err := os.Symlink(within, link); err != nil {
		t.Fatal(err)
	}
	if body, err := runfile.ReadRegular(within, 8); err != nil || len(body) != 8 {
		t.Errorf("a file at the bound: %q, %v", body, err)
	}
	for name, path := range map[string]string{"past the bound": past, "a link": link, "a directory": dir} {
		if _, err := runfile.ReadRegular(path, 8); err == nil {
			t.Errorf("a file that is %s was read", name)
		}
	}
}
