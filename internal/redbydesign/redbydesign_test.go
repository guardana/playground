package redbydesign

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseReadsAnIDItsChecksAndItsFindingPerLine(t *testing.T) {
	list := "# comment\n\nmode-01-observe evidence/executed-digest  the enforcer completes a refused call\n" +
		"verify-04-drift\tverifier/step-2/finding/guardana.agent.mcp_server_manifest,verifier/step-2/exit-code" +
		"\tthe verifier reports CRITICAL, the catalogue HIGH\n"
	got, err := Parse(strings.NewReader(list), "list")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []Entry{
		{
			ID: "mode-01-observe", Checks: []string{"evidence/executed-digest"},
			Finding: "the enforcer completes a refused call",
		},
		{
			ID:      "verify-04-drift",
			Checks:  []string{"verifier/step-2/finding/guardana.agent.mcp_server_manifest", "verifier/step-2/exit-code"},
			Finding: "the verifier reports CRITICAL, the catalogue HIGH",
		},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].ID != want[i].ID || got[i].Finding != want[i].Finding || !slices.Equal(got[i].Checks, want[i].Checks) {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseRefusesWhatItCannotReadAsOneScenarioItsChecksAndOneFinding(t *testing.T) {
	for name, list := range map[string]string{
		"no check and no finding":    "mode-01-observe\n",
		"blank after the id":         "mode-01-observe   \t \n",
		"no check field":             "mode-01 the enforcer completes a refused call\n",
		"no finding":                 "mode-01 evidence/executed-digest\n",
		"an empty check":             "mode-01 evidence/executed-digest,,boot/victim-fs the finding\n",
		"a trailing comma":           "mode-01 evidence/executed-digest, the finding\n",
		"a leading comma":            "mode-01 ,evidence/executed-digest the finding\n",
		"a check with an empty part": "mode-01 evidence//executed-digest the finding\n",
		"a check ending in a slash":  "mode-01 evidence/ the finding\n",
		"a check without a family":   "mode-01 /executed-digest the finding\n",
		"a check with odd bytes":     "mode-01 evidence/executed*digest the finding\n",
		"a check named twice":        "mode-01 evidence/executed-digest,evidence/executed-digest the finding\n",
		"a path, not an id":          "scenarios/mode/mode-01.yaml evidence/executed-digest the finding\n",
		"a file name":                "mode-01.yaml evidence/executed-digest the finding\n",
		"upper case":                 "Mode-01 evidence/executed-digest the finding\n",
		"listed twice": "mode-01 evidence/executed-digest one finding\n" +
			"mode-01 boot/victim-fs another finding\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := Parse(strings.NewReader(list), "list"); err == nil {
				t.Errorf("Parse(%q) = %+v, want an error", list, got)
			}
		})
	}
}

func TestParseNamesTheLineItRefuses(t *testing.T) {
	_, err := Parse(strings.NewReader("mode-01 evidence/executed-digest finding\n\nverify-04 a finding\n"), "red.txt")
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
	if err := os.WriteFile(path, []byte("rule-09 decisions/step-1 a finding\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil || len(got) != 1 || got[0].ID != "rule-09" || !slices.Equal(got[0].Checks, []string{"decisions/step-1"}) {
		t.Errorf("Read = %+v, %v", got, err)
	}
}
