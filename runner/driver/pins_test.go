package driver

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/guardana/playground/runner/report"
)

func writePins(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "versions.env")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadPinsKeepsTheFileOrder(t *testing.T) {
	path := writePins(t, "# comment\n\nB_PIN=two\nA_PIN=python:3.13-slim\nEMPTY=\n")
	pins, err := ReadPins(path)
	if err != nil {
		t.Fatalf("ReadPins: %v", err)
	}
	want := Pins{{Name: "B_PIN", Value: "two"}, {Name: "A_PIN", Value: "python:3.13-slim"}, {Name: "EMPTY", Value: ""}}
	if !slices.Equal(pins, want) {
		t.Errorf("ReadPins = %v, want %v", pins, want)
	}
}

func TestReadPinsRefusesALineThatIsNotAPin(t *testing.T) {
	for _, body := range []string{"export A=1\n", "a=1\n", "A 1\n", "A=1\nB\n"} {
		if pins, err := ReadPins(writePins(t, body)); err == nil {
			t.Errorf("ReadPins(%q) = %v, want a refusal", body, pins)
		}
	}
}

func TestAPinNobodyNamedReadsEmpty(t *testing.T) {
	pins := Pins{report.Pin{Name: "A_PIN", Value: "one"}}
	if got := pins.Value("A_PIN"); got != "one" {
		t.Errorf("A_PIN = %q, want one", got)
	}
	if got := pins.Value("B_PIN"); got != "" {
		t.Errorf("a pin versions.env does not name reads %q", got)
	}
}
