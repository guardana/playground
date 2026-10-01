package repoguard_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A name grep could read as an option is read as a file name: a root file named
// like one must not switch a guard's content scan off.
func TestANameLikeAnOptionDoesNotSilenceAGuard(t *testing.T) {
	for script, planted := range map[string]struct{ name, content, reason string }{
		"check-hygiene.sh":     {"probe.md", "Release date: TB" + "D\n", "placeholder: probe.md:1:"},
		"check-attribution.sh": {"probe.md", "Co-Authored" + "-By: A Tool <tool@example.com>\n", "probe.md:1:"},
		"check-file-sizes.sh":  {"-q.go", strings.Repeat("var _ = 0\n", 501), "FAIL -q.go: 501 lines"},
	} {
		t.Run(script, func(t *testing.T) {
			dir := t.TempDir()
			git(t, dir, "init", "-q")
			copyScripts(t, dir)
			for name, content := range map[string]string{"-q.md": "notes\n", planted.name: planted.content} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			command := exec.Command("./scripts/" + script) // #nosec G204 -- script names from the literal list.
			command.Dir = dir
			out, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(out), planted.reason) {
				t.Errorf("%s beside a file named -q.md: %v, want %q in\n%s", script, err, planted.reason, out)
			}
		})
	}
}

// A list that failed must stop the format check, which would otherwise hand
// gofmt no file and read its standard input as the whole tree.
func TestTheFormatCheckFailsWhenTheListDoes(t *testing.T) {
	for name, planted := range map[string]string{
		"an unformatted file":      "",
		"a name the list refuses":  "probe\nname.go",
		"an unparsable file":       "unparsable.go",
		"a file named like a flag": "-cpuprofile=profile.go",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			git(t, dir, "init", "-q")
			copyScripts(t, dir)
			body, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
			if err != nil {
				t.Fatal(err)
			}
			files := map[string]string{"Makefile": string(body), "unformatted.go": "package p\nvar  x = 1\n"}
			switch planted {
			case "":
			case "unparsable.go":
				files["unformatted.go"] = "package p\n\nvar x = 1\n"
				files[planted] = "package p\nfunc(\n"
			case "-cpuprofile=profile.go":
				files[planted] = files["unformatted.go"]
				files["unformatted.go"] = "package p\n\nvar x = 1\n"
			default:
				files["unformatted.go"] = "package p\n\nvar x = 1\n"
				files[planted] = "package p\n"
			}
			for file, content := range files {
				if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			command := exec.Command("make", "fmt-check") // #nosec G204 -- fixed arguments.
			command.Dir = dir
			if out, err := command.CombinedOutput(); err == nil {
				t.Errorf("fmt-check passed: %s", out)
			}
			if _, err := os.Stat(filepath.Join(dir, "profile.go")); err == nil {
				t.Error("gofmt took a file name for a flag and wrote profile.go")
			}
		})
	}
}

// grep skips a file it takes for binary under -I and one it cannot read, and a
// guard that swallows grep's errors would report either clean.
func TestAGuardReadsEveryListedFile(t *testing.T) {
	for name, probe := range map[string]struct {
		script, content, reason string
		mode                    os.FileMode
	}{
		"hygiene, a NUL before a placeholder": {"check-hygiene.sh", "\x00\nRelease date: TB" + "D\n", "placeholder: probe.md:2:", 0o600},
		"attribution, a NUL before a credit":  {"check-attribution.sh", "\x00\nCo-Authored" + "-By: A Tool <tool@example.com>\n", "probe.md:2:", 0o600},
		"hygiene, a file it cannot read":      {"check-hygiene.sh", "Release date: TB" + "D\n", "unreadable: probe.md", 0o000},
		"attribution, a file it cannot read":  {"check-attribution.sh", "notes\n", "unreadable: probe.md", 0o000},
		// GNU grep under a UTF-8 locale holds back a matching line that is not
		// UTF-8 unless it reads the file as text.
		"hygiene, a byte outside UTF-8 beside a placeholder": {"check-hygiene.sh", "Release date: TB" + "D \xff\n", "placeholder: probe.md:1:", 0o600},
	} {
		t.Run(name, func(t *testing.T) {
			if probe.mode == 0 && os.Getuid() == 0 {
				t.Skip("root reads a file of mode 000")
			}
			dir := t.TempDir()
			git(t, dir, "init", "-q")
			copyScripts(t, dir)
			if err := os.WriteFile(filepath.Join(dir, "probe.md"), []byte(probe.content), probe.mode); err != nil {
				t.Fatal(err)
			}
			command := exec.Command("./scripts/" + probe.script) // #nosec G204 -- script names from the literal list.
			command.Dir = dir
			command.Env = append(os.Environ(), "LC_ALL=C.UTF-8")
			out, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(out), probe.reason) {
				t.Errorf("%v, want %q in\n%s", err, probe.reason, out)
			}
		})
	}
}
