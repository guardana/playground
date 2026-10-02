package labspec_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/internal/labspec"
)

func TestCommittedIsReadValueForValue(t *testing.T) {
	s, tr := loadPair(t, withFSEffects(
		`{ calls_served: { fs.read: 1 }, committed: { fs.read: [ { path: "/data/x", bytes: 512, cached: false } ] } }`))
	if err := labspec.Validate(s, tr); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	got := s.Expect.Effects["victim-fs"].Committed["fs.read"]
	want := journal.Effect{"path": journal.String("/data/x"), "bytes": journal.Integer(512), "cached": journal.Bool(false)}
	if len(got) != 1 || !got[0].Equal(want) {
		t.Errorf("committed = %v, want [%s]", got, want)
	}
}

// Each case is a statement no run can meet, or one that cannot be compared
// exactly with what a victim wrote.
func TestCommittedRefusesAStatementThatCannotPass(t *testing.T) {
	for name, entry := range map[string]string{
		"an empty list":           `{ calls_served: { fs.read: 1 }, committed: { fs.read: [] } }`,
		"an empty object":         `{ calls_served: { fs.read: 1 }, committed: { fs.read: [ {} ] } }`,
		"a tool with no name":     `{ calls_served: { fs.read: 1 }, committed: { "": [ { amount: 1 } ] } }`,
		"fewer than served":       `{ calls_served: { fs.read: 2 }, committed: { fs.read: [ { amount: 1 } ] } }`,
		"more than served":        `{ calls_served: { fs.read: 1 }, committed: { fs.read: [ { amount: 1 }, { amount: 2 } ] } }`,
		"a tool never served":     `{ calls_served: { fs.read: 1 }, committed: { fs.write: [ { amount: 1 } ] } }`,
		"a fraction":              `{ calls_served: { fs.read: 1 }, committed: { fs.read: [ { amount: 5000.5 } ] } }`,
		"past 2^53-1":             `{ calls_served: { fs.read: 1 }, committed: { fs.read: [ { amount: 9007199254740993 } ] } }`,
		"a null":                  `{ calls_served: { fs.read: 1 }, committed: { fs.read: [ { amount: null } ] } }`,
		"a nested object":         `{ calls_served: { fs.read: 1 }, committed: { fs.read: [ { amount: { value: 1 } } ] } }`,
		"an unknown key":          `{ calls_served: { fs.read: 1 }, commits: { fs.read: [ { amount: 1 } ] } }`,
		"a string past 256 bytes": `{ calls_served: { fs.read: 1 }, committed: { fs.read: [ { s: "` + strings.Repeat("s", 257) + `" } ] } }`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := labspec.LoadScenario(writeFile(t, "case-01-allow-read.yaml", withFSEffects(entry)))
			if err == nil {
				t.Fatal("loaded")
			}
			if !errors.Is(err, labspec.ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

// The largest integer both sides hold exactly loads; the bound is not below it.
// YAML itself writes 5000.0 as the integer 5000 before the decoder sees it,
// which is exact, so it loads as 5000.
func TestCommittedTakesTheIntegersYAMLWritesExactly(t *testing.T) {
	for text, want := range map[string]int64{"9007199254740991": 1<<53 - 1, "5000.0": 5000, "-1": -1} {
		s, err := labspec.LoadScenario(writeFile(t, "case-01-allow-read.yaml", withFSEffects(
			`{ calls_served: { fs.read: 1 }, committed: { fs.read: [ { amount: `+text+` } ] } }`)))
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		got := s.Expect.Effects["victim-fs"].Committed["fs.read"][0]["amount"]
		if !got.Equal(journal.Integer(want)) {
			t.Errorf("%s loaded as %s, want %d", text, got, want)
		}
	}
}

func TestCommittedIsRefusedUnderADouble(t *testing.T) {
	body := strings.Replace(withDouble("approver", "approvals", "approver_script"),
		"approver: { calls_served: { approve: 1 } }",
		"approver: { calls_served: { approve: 1 }, committed: { approve: [ { amount: 1 } ] } }", 1)
	_, err := labspec.LoadScenario(writeFile(t, "case-01-allow-read.yaml", body))
	if !errors.Is(err, labspec.ErrInvalid) || !strings.Contains(err.Error(), "double") {
		t.Errorf("err = %v, want ErrInvalid naming the double", err)
	}
}
