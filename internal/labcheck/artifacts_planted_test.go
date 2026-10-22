package labcheck_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func plant(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func wantOneProblem(t *testing.T, problems []string, fragment string) {
	t.Helper()
	if len(problems) != 1 || !strings.Contains(problems[0], fragment) {
		t.Errorf("got %q, want one problem naming %q", problems, fragment)
	}
}

func TestTheArtifactGuardRefusesAPlantedFile(t *testing.T) {
	cases := map[string]struct{ name, data, want string }{
		"a byte outside ASCII":   {"case/control/a.yaml", "name: caf" + string([]byte{0xc3, 0xa9}) + "\n", "byte 0xc3 at offset 9"},
		"a control character":    {"case/control/a.yaml", "a: b\r\n", "byte 0x0d"},
		"a resolvable host":      {"case/control/a.json", `{"url":"https://hub.` + `example.org/x"}`, "names no host under .invalid"},
		"a host ending invalid":  {"case/control/a.md", "see http://lab-invalid/x", "names no host under .invalid"},
		"a pickle that calls":    {"case/defect/m.pkl", "c" + absentModule + "\nnothing\n)" + "R.", "REDUCE"},
		"a pickle that imports":  {"case/defect/m.pkl", "c" + "o" + "s\nsystem\n.", "GLOBAL imports"},
		"an unjudged kind":       {"case/defect/m.bin", "data", "no guard judges"},
		"a loadable kernel repo": {"case/defect/config.json", `{"_attn_implementation":"lab-org/kernel"}`, "a repository the hub client would fetch"},
		"a loadable auto_map": {"case/defect/config.json", `{"auto_map":{"AutoModel":"lab-org/repo--modeling.Model"}}`,
			"a repository the hub client would fetch"},
		"code at module level": {"case/defect/train.py", "\"\"\"Doc.\"\"\"\nimport json\n", "runs on import"},
		"a file a tool runs":   {"case/defect/conftest.py", "def f():\n    pass\n", "a name tooling runs"},
	}
	for label, c := range cases {
		t.Run(label, func(t *testing.T) {
			root := t.TempDir()
			plant(t, root, map[string]string{c.name: c.data})
			problems, files := artifactFileProblems(root)
			if files != 1 {
				t.Fatalf("inspected %d files, want 1", files)
			}
			wantOneProblem(t, problems, c.want)
		})
	}
}

func TestTheArtifactGuardTakesInertText(t *testing.T) {
	root := t.TempDir()
	plant(t, root, map[string]string{
		"case/control/a.json":     `{"url":"https://hub.lab.invalid:8443/x?y=1"}` + "\n",
		"case/defect/config.json": `{"_attn_implementation":"lab--invalid/kernel"}` + "\n",
		"case/defect/train.py":    "\"\"\"A docstring\nover two lines.\"\"\"\n\n# A comment.\ndef train():\n    import json\n    return json\n",
		"case/defect/m.pkl":       "c" + absentModule + "\nnothing\n.",
		"README.md":               "Tabs\tand lines.\n",
	})
	problems, files := artifactFileProblems(root)
	if files != 5 || len(problems) != 0 {
		t.Errorf("inspected %d files with problems %q, want 5 and none", files, problems)
	}
}

func TestTheArtifactGuardRefusesALink(t *testing.T) {
	root := t.TempDir()
	plant(t, root, map[string]string{"target.yaml": "a: b\n"})
	if err := os.Symlink("target.yaml", filepath.Join(root, "link.yaml")); err != nil {
		t.Fatal(err)
	}
	problems, _ := artifactFileProblems(root)
	wantOneProblem(t, problems, "link.yaml: not a regular file")
}

func TestTheCaseGuardRefusesVariantsThatDoNotDifferInOneFile(t *testing.T) {
	same := map[string]string{"case/control/a.yaml": "a\n", "case/defect/a.yaml": "a\n"}
	cases := map[string]struct {
		files map[string]string
		want  string
	}{
		"identical": {same, "differ in [], want exactly one"},
		"two files differ": {map[string]string{
			"case/control/a.yaml": "a\n", "case/defect/a.yaml": "b\n",
			"case/control/b.yaml": "a\n", "case/defect/b.yaml": "b\n",
		}, "differ in [a.yaml b.yaml]"},
		"a file only in defect": {map[string]string{
			"case/control/a.yaml": "a\n", "case/defect/a.yaml": "b\n", "case/defect/sub/c.yaml": "c\n",
		}, "defect holds [a.yaml sub/c.yaml]"},
		"a third variant": {map[string]string{
			"case/control/a.yaml": "a\n", "case/defect/a.yaml": "b\n", "case/other/a.yaml": "c\n",
		}, "want exactly control and defect"},
		"no defect": {map[string]string{"case/control/a.yaml": "a\n"}, "want exactly control and defect"},
	}
	for label, c := range cases {
		t.Run(label, func(t *testing.T) {
			root := t.TempDir()
			plant(t, root, c.files)
			problems, cases := caseProblems(root)
			if cases != 1 {
				t.Fatalf("read %d cases, want 1", cases)
			}
			wantOneProblem(t, problems, c.want)
		})
	}
}

func TestTheCaseGuardTakesVariantsThatDifferInOneFile(t *testing.T) {
	root := t.TempDir()
	plant(t, root, map[string]string{
		"case/control/a.yaml": "a\n", "case/defect/a.yaml": "b\n",
		"case/control/sub/b.yaml": "same\n", "case/defect/sub/b.yaml": "same\n",
		"README.md": "not a case\n",
	})
	problems, cases := caseProblems(root)
	if cases != 1 || len(problems) != 0 {
		t.Errorf("read %d cases with problems %q, want 1 and none", cases, problems)
	}
}

func TestTheAbsentModuleGuardRefusesAnythingPythonCouldImport(t *testing.T) {
	refused := []string{
		absentModule + ".py",
		"lib/" + absentModule + "/__init__.py",
		"lib/" + absentModule + ".cpython-312.pyc",
		"lib/" + absentModule + ".so",
	}
	for _, file := range refused {
		wantOneProblem(t, absentModuleProblems([]string{file}), file+" provides")
	}
	taken := []string{"lib/" + absentModule + "_too.py", "artifacts/pickle-import/defect/model.pkl"}
	if problems := absentModuleProblems(taken); len(problems) != 0 {
		t.Errorf("refused %q", problems)
	}
}
