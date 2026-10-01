package runfile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// readWithin fails the test when the read has not returned in time: an open
// that blocks on a FIFO never returns on its own.
func readWithin(t *testing.T, path string, swap func()) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := readRegular(path, 64, swap)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("the read blocked on what was swapped in after the check")
		return nil
	}
}

func fifo(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
}

// A container can replace the file between the runner's check and its open.
// What it swaps in is neither waited on nor followed.
func TestWhatIsSwappedInAfterTheCheckIsRefusedWithoutBlocking(t *testing.T) {
	for name, plant := range map[string]func(t *testing.T, dir, path string){
		"a FIFO": func(t *testing.T, _, path string) { fifo(t, path) },
		"a link to a FIFO": func(t *testing.T, dir, path string) {
			target := filepath.Join(dir, "pipe")
			fifo(t, target)
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "journal.jsonl")
			if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			swap := func() {
				if err := os.Remove(path); err != nil {
					t.Error(err)
				}
				plant(t, dir, path)
			}
			if err := readWithin(t, path, swap); err == nil {
				t.Errorf("%s swapped in after the check was read", name)
			}
		})
	}
}
