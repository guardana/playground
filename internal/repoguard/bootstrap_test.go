package repoguard_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A download whose sha256 is not the pinned one is refused from inside fetch,
// and the refusal exits the script; what fetch downloaded goes with it.
func TestARefusedDownloadLeavesNothingBehind(t *testing.T) {
	stubs, scratch := t.TempDir(), t.TempDir()
	marker := filepath.Join(stubs, "curl-ran")
	curl := "#!/bin/sh\ntouch '" + marker + "'\n" +
		"while [ $# -gt 0 ]; do\n\tif [ \"$1\" = -o ]; then printf 'not the release' >\"$2\"; exit 0; fi\n\tshift\ndone\nexit 1\n"
	if err := os.WriteFile(filepath.Join(stubs, "curl"), []byte(curl), 0o700); err != nil { // #nosec G306 -- a stub to run.
		t.Fatal(err)
	}
	// #nosec G204 -- fixed arguments.
	command := exec.Command("bash", "-c",
		`source scripts/bootstrap.sh && source scripts/tool-versions.env && fetch gitleaks linux amd64 "$1"`,
		"bootstrap", filepath.Join(t.TempDir(), "bin"))
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "PATH="+stubs+string(os.PathListSeparator)+os.Getenv("PATH"), "TMPDIR="+scratch)
	out, err := command.CombinedOutput()
	if _, stat := os.Stat(marker); stat != nil {
		t.Fatalf("the stub curl did not run: %s", out)
	}
	if err == nil || !strings.Contains(string(out), "sha256 mismatch") || strings.Contains(string(out), "unbound") {
		t.Fatalf("fetch of a download no pin matches: %v\n%s", err, out)
	}
	left, err := os.ReadDir(scratch)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range left {
		t.Errorf("left behind in TMPDIR: %s", entry.Name())
	}
}

// A platform with no pin or no asset would make bootstrap refuse on a
// stranger's machine that CI never runs on.
func TestEveryToolHasAPinAndAnAssetOnEveryPlatform(t *testing.T) {
	// #nosec G204 -- fixed arguments.
	command := exec.Command("bash", "-c", `source scripts/bootstrap.sh && source scripts/tool-versions.env &&
		for tool in $TOOLS; do for os in linux darwin; do for arch in amd64 arm64; do
			printf '%s %s %s %s %s\n' "$tool" "$os" "$arch" "$(pinned "$tool" "SHA256_$(upper "$os")_$(upper "$arch")")" "$(asset_url "$tool" "$os" "$arch")"
		done; done; done`)
	command.Dir = repoRoot
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 5*2*2 {
		t.Fatalf("want 20 platform lines, got %d:\n%s", len(lines), out)
	}
	line := regexp.MustCompile(`^\S+ (linux|darwin) (amd64|arm64) [0-9a-f]{64} https://github\.com/\S+$`)
	for _, l := range lines {
		if !line.MatchString(l) {
			t.Errorf("no pin or no asset: %s", l)
		}
	}
}

// A Go other than the one go.mod names compiles, vets and lints another gate.
func TestTheGoVersionCheckRefusesAnotherGo(t *testing.T) {
	want := goModVersion(t)
	for _, c := range []struct {
		name, stub string
		passes     bool
	}{
		{"the named version", "echo " + want, true},
		{"a newer Go", "echo go99.0.0", false},
		{"a Go that cannot run in the module", "echo 'go.mod requires a newer go' >&2; exit 1", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, err := checkGoVersionWith(t, c.stub)
			if c.passes && err != nil {
				t.Fatalf("refused %s: %s", want, out)
			}
			if !c.passes && (err == nil || !strings.Contains(out, "GOTOOLCHAIN="+want)) {
				t.Fatalf("passed, or refused without naming the fix: %v\n%s", err, out)
			}
		})
	}
}

// goModVersion reads go.mod's go line itself, apart from the script it checks.
func goModVersion(t *testing.T) string {
	t.Helper()
	mod, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(mod), "\n") {
		if fields := strings.Fields(l); len(fields) == 2 && fields[0] == "go" {
			return "go" + fields[1]
		}
	}
	t.Fatal("go.mod names no go version")
	return ""
}

// checkGoVersionWith runs the check with a go first on PATH whose
// `env GOVERSION` runs stub.
func checkGoVersionWith(t *testing.T, stub string) (string, error) {
	t.Helper()
	stubs := t.TempDir()
	script := "#!/bin/sh\n[ \"$1 $2\" = 'env GOVERSION' ] || exit 2\n" + stub + "\n"
	if err := os.WriteFile(filepath.Join(stubs, "go"), []byte(script), 0o700); err != nil { // #nosec G306 -- a stub to run.
		t.Fatal(err)
	}
	command := exec.Command("./scripts/check-go-version.sh")
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "PATH="+stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := command.CombinedOutput()
	return string(out), err
}

// A binary already in ./bin is kept only when its bytes are the verified
// download's: one that merely reports the pinned version is replaced.
func TestABinaryInBinIsReplacedUnlessItIsTheVerifiedOne(t *testing.T) {
	stubs, bin := t.TempDir(), t.TempDir()
	release := filepath.Join(t.TempDir(), "osv-scanner")
	if err := os.WriteFile(release, []byte("#!/bin/sh\necho 'osv-scanner version: 2.5.1'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	curl := "#!/bin/sh\nwhile [ $# -gt 0 ]; do\n\tif [ \"$1\" = -o ]; then cp \"$RELEASE\" \"$2\"; exit 0; fi\n\tshift\ndone\nexit 1\n"
	if err := os.WriteFile(filepath.Join(stubs, "curl"), []byte(curl), 0o700); err != nil { // #nosec G306 -- a stub to run.
		t.Fatal(err)
	}
	planted := "#!/bin/sh\necho 'osv-scanner version: 2.5.1'\necho planted\n"
	if err := os.WriteFile(filepath.Join(bin, "osv-scanner"), []byte(planted), 0o700); err != nil { // #nosec G306 -- a binary to replace.
		t.Fatal(err)
	}
	// #nosec G204 -- fixed arguments.
	command := exec.Command("bash", "-c", `source scripts/bootstrap.sh && source scripts/tool-versions.env &&
		OSV_SCANNER_SHA256_LINUX_AMD64=$(sha256_of "$RELEASE") && fetch osv-scanner linux amd64 "$1"`, "bootstrap", bin)
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "RELEASE="+release, "TMPDIR="+t.TempDir(),
		"PATH="+stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fetch: %v\n%s", err, out)
	}
	got, err := os.ReadFile(filepath.Join(bin, "osv-scanner"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(release)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("./bin kept %q, want the verified download", got)
	}
}

// The gate's check reads BIN alone and names the fix when a tool is missing.
func TestTheGateRefusesABinWithoutThePinnedTools(t *testing.T) {
	out, err := verifyBin(t, t.TempDir())
	if err == nil || !strings.Contains(out, "run make bootstrap") {
		t.Fatalf("an empty BIN passed, or was refused without the fix: %v\n%s", err, out)
	}
}

// Binaries that print the pinned versions are not the tools bootstrap
// verified: without its record the gate refuses them, and runs none of them.
func TestTheGateRunsNoBinaryBootstrapDidNotRecord(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	manifest := ""
	for tool, says := range map[string]string{
		"golangci-lint": "golangci-lint has version 2.13.2", "actionlint": "1.7.12", "gitleaks": "8.30.1",
		"osv-scanner": "osv-scanner version: 2.5.1", "zizmor": "zizmor 1.30.1",
	} {
		body := "#!/bin/sh\ntouch '" + marker + "'\necho '" + says + "'\n"
		if err := os.WriteFile(filepath.Join(bin, tool), []byte(body), 0o700); err != nil { // #nosec G306 -- a stub to run.
			t.Fatal(err)
		}
		manifest += fmt.Sprintf("%s %x\n", tool, sha256.Sum256([]byte(body)))
	}
	if out, err := verifyBin(t, bin); err == nil || !strings.Contains(out, "not the binary bootstrap verified") {
		t.Fatalf("unrecorded binaries passed: %v\n%s", err, out)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the gate ran a binary bootstrap did not record")
	}
	if err := os.WriteFile(filepath.Join(bin, ".verified"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := verifyBin(t, bin); err != nil {
		t.Fatalf("recorded binaries at the pinned versions were refused: %v\n%s", err, out)
	}
}

// The verified bytes without the execute bit are installed again.
func TestBootstrapRepairsAVerifiedBinaryThatCannotRun(t *testing.T) {
	stubs, bin := t.TempDir(), t.TempDir()
	release := filepath.Join(t.TempDir(), "osv-scanner")
	body := []byte("#!/bin/sh\necho 'osv-scanner version: 2.5.1'\n")
	if err := os.WriteFile(release, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "osv-scanner"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	curl := "#!/bin/sh\nwhile [ $# -gt 0 ]; do\n\tif [ \"$1\" = -o ]; then cp \"$RELEASE\" \"$2\"; exit 0; fi\n\tshift\ndone\nexit 1\n"
	if err := os.WriteFile(filepath.Join(stubs, "curl"), []byte(curl), 0o700); err != nil { // #nosec G306 -- a stub to run.
		t.Fatal(err)
	}
	// #nosec G204 -- fixed arguments.
	command := exec.Command("bash", "-c", `source scripts/bootstrap.sh && source scripts/tool-versions.env &&
		OSV_SCANNER_SHA256_LINUX_AMD64=$(sha256_of "$RELEASE") && fetch osv-scanner linux amd64 "$1"`, "bootstrap", bin)
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "RELEASE="+release, "TMPDIR="+t.TempDir(),
		"PATH="+stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fetch: %v\n%s", err, out)
	}
	if info, err := os.Stat(filepath.Join(bin, "osv-scanner")); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("the binary is %v (%v), want executable", info, err)
	}
}

func verifyBin(t *testing.T, bin string) (string, error) {
	t.Helper()
	command := exec.Command("scripts/bootstrap.sh", "--verify")
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "BIN="+bin)
	out, err := command.CombinedOutput()
	return string(out), err
}
