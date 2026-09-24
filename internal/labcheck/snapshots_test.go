package labcheck_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// snapshotDigests is written by scripts/classify-victims.sh in the same run as
// fingerprints.yaml, one `<sha256>  tools/<victim>.json` line per snapshot.
const snapshotDigests = "config/gateway/tools.sha256"

// The fingerprints were printed for the definitions in the listing snapshots.
// A snapshot rewritten on its own (`go test ./victims/... -update`) leaves the
// fingerprints pinning definitions no victim lists any more.
func TestTheFingerprintsWereTakenFromTheseSnapshots(t *testing.T) {
	recorded := readDigests(t)
	paths, err := filepath.Glob(filepath.Join(repoRoot, "config", "gateway", "tools", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no listing snapshot: %v", err)
	}
	for _, path := range paths {
		name := "tools/" + filepath.Base(path)
		body, err := os.ReadFile(path) // #nosec G304 -- the lab's own snapshots.
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		switch want, found := recorded[name]; {
		case !found:
			t.Errorf("%s has no digest in %s; run scripts/classify-victims.sh", name, snapshotDigests)
		case want != hex.EncodeToString(sum[:]):
			t.Errorf("%s changed since its fingerprints were taken; run scripts/classify-victims.sh", name)
		}
		delete(recorded, name)
	}
	for name := range recorded {
		t.Errorf("%s names %s, which is no snapshot", snapshotDigests, name)
	}
}

func readDigests(t *testing.T) map[string]string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repoRoot, snapshotDigests)) // #nosec G304 -- the lab's own file.
	if err != nil {
		t.Fatalf("%s: %v; run scripts/classify-victims.sh", snapshotDigests, err)
	}
	recorded := map[string]string{}
	for number, line := range strings.Split(strings.TrimSuffix(string(body), "\n"), "\n") {
		sum, name, found := strings.Cut(line, "  ")
		if !found || len(sum) != sha256.Size*2 || !strings.HasPrefix(name, "tools/") {
			t.Fatalf("%s:%d is not `<sha256>  tools/<victim>.json`: %q", snapshotDigests, number+1, line)
		}
		if _, twice := recorded[name]; twice {
			t.Fatalf("%s names %s twice", snapshotDigests, name)
		}
		recorded[name] = sum
	}
	return recorded
}
