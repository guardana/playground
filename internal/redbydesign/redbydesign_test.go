package redbydesign

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseReadsAnIDAndItsFindingPerLine(t *testing.T) {
	list := "# comment\n\nmode-01-observe  the enforcer completes a refused call\n" +
		"verify-04-drift\tthe verifier reports CRITICAL, the catalogue HIGH\n"
	got, err := Parse(strings.NewReader(list), "list")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []Entry{
		{ID: "mode-01-observe", Finding: "the enforcer completes a refused call"},
		{ID: "verify-04-drift", Finding: "the verifier reports CRITICAL, the catalogue HIGH"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseRefusesWhatItCannotReadAsOneScenarioAndOneFinding(t *testing.T) {
	for name, list := range map[string]string{
		"no finding":        "mode-01-observe\n",
		"blank finding":     "mode-01-observe   \t \n",
		"a path, not an id": "scenarios/mode/mode-01.yaml the finding\n",
		"a file name":       "mode-01.yaml the finding\n",
		"upper case":        "Mode-01 the finding\n",
		"listed twice":      "mode-01 one finding\nmode-01 another finding\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := Parse(strings.NewReader(list), "list"); err == nil {
				t.Errorf("Parse(%q) = %+v, want an error", list, got)
			}
		})
	}
}

func TestParseNamesTheLineItRefuses(t *testing.T) {
	_, err := Parse(strings.NewReader("mode-01 finding\n\nverify-04\n"), "red.txt")
	if err == nil || !strings.Contains(err.Error(), "red.txt:3") {
		t.Errorf("err = %v, want it to name red.txt:3", err)
	}
}

func TestReadFailsOnAMissingList(t *testing.T) {
	if _, err := Read(filepath.Join(t.TempDir(), "absent.txt")); err == nil {
		t.Error("a missing list read as an empty one")
	}
}

func TestReadReadsAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "red.txt")
	if err := os.WriteFile(path, []byte("rule-09 a finding\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil || len(got) != 1 || got[0].ID != "rule-09" {
		t.Errorf("Read = %+v, %v", got, err)
	}
}
