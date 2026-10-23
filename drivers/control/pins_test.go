package control

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/guardana/playground/runner/driver"
)

func readPins(t *testing.T, root string) driver.Pins {
	t.Helper()
	pins, err := driver.ReadPins(filepath.Join(root, versionFile))
	if err != nil {
		t.Fatal(err)
	}
	return pins
}

// writeFile writes a fixture, making its directory.
func writeFile(path, body string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o750)
	_ = os.WriteFile(path, []byte(body), 0o600)
}

func TestTheNamespaceIsReadFromThePins(t *testing.T) {
	root := t.TempDir()
	writeFile(filepath.Join(root, versionFile), "ENFORCER_COMMIT=abc\nENFORCER_NAMESPACE=guardana.control\n")
	if got, err := enforcerNamespace(readPins(t, root)); err != nil || got != "guardana.control" {
		t.Errorf("enforcerNamespace = %q, %v", got, err)
	}
	writeFile(filepath.Join(root, versionFile), "ENFORCER_COMMIT=abc\n")
	if got, err := enforcerNamespace(readPins(t, root)); err == nil {
		t.Errorf("a versions.env without the namespace gave %q", got)
	}
}
