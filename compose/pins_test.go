package compose

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// readers are the files that build or run the lab. A pin none of them reads is
// a version nobody tests, and a header that says otherwise.
var readers = []string{"compose.yaml", "Dockerfile.*", "../scripts/*.sh", "../runner/*.go", "../Makefile"}

func pins(t *testing.T) map[string]string {
	t.Helper()
	file, err := os.Open("../versions.env")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	found := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("versions.env: %q is not NAME=VALUE", line)
		}
		found[name] = value
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(found) == 0 {
		t.Fatal("versions.env sets no variable")
	}
	return found
}

func readersText(t *testing.T) string {
	t.Helper()
	var text strings.Builder
	for _, pattern := range readers {
		matched, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(matched) == 0 {
			t.Fatalf("no file matches %s, so no pin can be read there", pattern)
		}
		for _, path := range matched {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			body, err := os.ReadFile(path) // #nosec G304 -- the lab's own files.
			if err != nil {
				t.Fatal(err)
			}
			text.WriteString(withoutComments(string(body)))
		}
	}
	return text.String()
}

// withoutComments drops whole comment lines, so a pin that is only mentioned
// in prose does not count as read.
func withoutComments(body string) string {
	var kept strings.Builder
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		kept.WriteString(line + "\n")
	}
	return kept.String()
}

func reads(text, name string) bool {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`).MatchString(text)
}

func TestEveryPinIsRead(t *testing.T) {
	text := readersText(t)
	for name := range pins(t) {
		if !reads(text, name) {
			t.Errorf("versions.env sets %s and nothing that builds or runs the lab reads it", name)
		}
	}
}

func TestAPinNamedOnlyInACommentIsNotRead(t *testing.T) {
	name := "PLANTED" + "_PIN"
	for _, body := range []string{
		"# builds from " + name + "\nFROM scratch\n",
		"\t// the report prints " + name + "\nvar x = 1\n",
		"  #   " + name + " is set by the caller\n",
	} {
		if reads(withoutComments(body), name) {
			t.Errorf("%q counts as reading %s", body, name)
		}
	}
	if !reads(withoutComments("image=\"$(pin "+name+")\"\n"), name) {
		t.Errorf("a line that reads %s is not counted", name)
	}
}

func TestTheVerifierLockPinsTheRelease(t *testing.T) {
	set := pins(t)
	want := set["VERIFIER_PACKAGE"] + "==" + set["VERIFIER_VERSION"]
	if want == "==" {
		t.Fatal("versions.env names no verifier package or version")
	}
	lock, err := os.ReadFile("verifier/requirements.lock")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(want) + ` \\$`).Match(lock) {
		t.Errorf("verifier/requirements.lock does not pin %s", want)
	}
	input, err := os.ReadFile("verifier/requirements.in")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(want) + `$`).Match(input) {
		t.Errorf("verifier/requirements.in does not ask for %s", want)
	}
}

// Every requirement in the lock carries at least one hash, which is what
// --require-hashes installs against.
func TestEveryLockedRequirementCarriesAHash(t *testing.T) {
	lock, err := os.ReadFile("verifier/requirements.lock")
	if err != nil {
		t.Fatal(err)
	}
	requirement := regexp.MustCompile(`^[A-Za-z0-9._-]+==[^ ]+ \\$`)
	lines := strings.Split(string(lock), "\n")
	count := 0
	for i, line := range lines {
		if !requirement.MatchString(line) {
			continue
		}
		count++
		if i+1 >= len(lines) || !strings.HasPrefix(strings.TrimSpace(lines[i+1]), "--hash=sha256:") {
			t.Errorf("%s carries no hash", strings.TrimSuffix(line, " \\"))
		}
	}
	if count == 0 {
		t.Fatal("verifier/requirements.lock pins no requirement")
	}
}

// Images the lab pulls are pinned by the digest of their multi-arch index; a
// tag alone can be moved under the lab. The lab's own images are built here.
func TestEveryPulledImageIsPinnedByDigest(t *testing.T) {
	built := map[string]bool{"ENFORCER_IMAGE": true, "VERIFIER_IMAGE": true}
	pinned := regexp.MustCompile(`^[a-z0-9][a-z0-9./_-]*:[A-Za-z0-9._-]+@sha256:[0-9a-f]{64}$`)
	count := 0
	for name, value := range pins(t) {
		if !strings.HasSuffix(name, "_IMAGE") || built[name] {
			continue
		}
		count++
		if !pinned.MatchString(value) {
			t.Errorf("%s=%s is not name:tag@sha256:<digest>", name, value)
		}
	}
	if count == 0 {
		t.Fatal("versions.env pins no image the lab pulls")
	}
}
