package check

import (
	"os"
	"path/filepath"
	"testing"
)

// The verifier writes its pin into a directory it can write, so a pin that is
// a link is its to plant: read through, it would hand the runner a host file
// or an endless one.
func TestAPinThatIsALinkIsNotRead(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "pin.json")
	if err := os.WriteFile(outside, []byte(`{"schema_version": 2, "server": "http://victim-fs:8080/mcp", "tools": {"fs.read": "sha256:00"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	pin := filepath.Join(dir, "pin-1.json")
	if err := os.Symlink(outside, pin); err != nil {
		t.Fatal(err)
	}
	if _, err := readPin(pin, "http://victim-fs:8080/mcp"); err == nil {
		t.Error("a pin that links elsewhere was read")
	}
	if read, err := readPin(outside, "http://victim-fs:8080/mcp"); err != nil || read.Tools["fs.read"] != "sha256:00" {
		t.Errorf("the same pin as a file was not read: %+v, %v", read, err)
	}
}
