package guardana

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(path, body string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o750)
	_ = os.WriteFile(path, []byte(body), 0o600)
}

// outsideFile is a file outside the run a planted link points at, and says
// whether it is still what it was.
func outsideFile(t *testing.T) (string, func() bool) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "host-file")
	writeFile(path, "untouched")
	return path, func() bool {
		body, err := os.ReadFile(path)
		return err == nil && string(body) == "untouched"
	}
}

func TestTheTraceIsTakenOnlyAsARegularFileWithinItsBound(t *testing.T) {
	dir := t.TempDir()
	outside, _ := outsideFile(t)
	within := filepath.Join(dir, "within.jsonl")
	writeFile(within, strings.Repeat("x", maxTraceBytes))
	past := filepath.Join(dir, "past.jsonl")
	writeFile(past, strings.Repeat("x", maxTraceBytes+1))
	link := filepath.Join(dir, "link.jsonl")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := copyTrace(within, filepath.Join(dir, "to-within")); err != nil {
		t.Errorf("a trace at the bound was refused: %v", err)
	}
	for name, from := range map[string]string{"past the bound": past, "a link": link, "a directory": dir} {
		if err := copyTrace(from, filepath.Join(dir, "to-"+strings.ReplaceAll(name, " ", "-"))); err == nil {
			t.Errorf("a trace that is %s was taken", name)
		}
	}
	for name, planted := range map[string]bool{"an empty directory": false, "a directory holding a link": true} {
		existing := filepath.Join(dir, strings.ReplaceAll(name, " ", "-"))
		if err := os.Mkdir(existing, 0o700); err != nil {
			t.Fatal(err)
		}
		if planted {
			if err := os.Symlink(outside, filepath.Join(existing, traceFile)); err != nil {
				t.Fatal(err)
			}
		}
		if err := copyTrace(within, existing); err == nil {
			t.Errorf("the trace was handed over into %s the runner did not make", name)
		}
	}
}
