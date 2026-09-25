package labspec_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

const fsServedLine = "    victim-fs: { calls_served: { fs.read: 1 } }\n"

func withFSEffects(entry string) string {
	return strings.Replace(goodScenario, fsServedLine, "    victim-fs: "+entry+"\n", 1)
}

func TestCallsRefusedIsReadBesideCallsServed(t *testing.T) {
	s, tr := loadPair(t, withFSEffects("{ calls_served: { fs.read: 1 }, calls_refused: { fs.read: 2, fs.list: 1 } }"))
	if err := labspec.Validate(s, tr); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	refused := s.Expect.Effects["victim-fs"].CallsRefused
	if refused["fs.read"] != 2 || refused["fs.list"] != 1 || len(refused) != 2 {
		t.Errorf("victim-fs calls_refused = %v, want fs.read=2 fs.list=1", refused)
	}
	if served := s.Expect.Effects["victim-fs"].CallsServed["fs.read"]; served != 1 {
		t.Errorf("victim-fs calls_served fs.read = %d, want 1", served)
	}
}

// Absent is the statement that nothing was refused, the same as an empty map.
func TestCallsRefusedAbsentMeansNoneRefused(t *testing.T) {
	for name, entry := range map[string]string{
		"absent": "{ calls_served: { fs.read: 1 } }",
		"empty":  "{ calls_served: { fs.read: 1 }, calls_refused: {} }",
	} {
		t.Run(name, func(t *testing.T) {
			s, tr := loadPair(t, withFSEffects(entry))
			if err := labspec.Validate(s, tr); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if refused := s.Expect.Effects["victim-fs"].CallsRefused; len(refused) != 0 {
				t.Errorf("calls_refused = %v, want none", refused)
			}
		})
	}
}

// A count below one states nothing absence would not, or a number of lines no
// journal can hold; a tool with no name is a line nothing can be matched to.
func TestCallsRefusedRefusesACountThatStatesNothing(t *testing.T) {
	for name, entry := range map[string]string{
		"zero":         "{ calls_served: { fs.read: 1 }, calls_refused: { fs.read: 0 } }",
		"negative":     "{ calls_served: { fs.read: 1 }, calls_refused: { fs.read: -1 } }",
		"unnamed tool": `{ calls_served: { fs.read: 1 }, calls_refused: { "": 1 } }`,
		"misspelled":   "{ calls_served: { fs.read: 1 }, calls_refuse: { fs.read: 1 } }",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := labspec.LoadScenario(writeFile(t, "case-01-allow-read.yaml", withFSEffects(entry)))
			if !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
			if name != "misspelled" && !strings.Contains(err.Error(), "victim-fs") {
				t.Errorf("error does not name the journal: %v", err)
			}
		})
	}
}

func TestCallsRefusedHoldsForADoubleAndAVerifierScenario(t *testing.T) {
	body := strings.Replace(withDouble("approver", "approvals", "approver_script"),
		"approver: { calls_served: { approve: 1 } }",
		"approver: { calls_served: { approve: 1 }, calls_refused: { deny: 0 } }", 1)
	if _, err := labspec.LoadScenario(writeFile(t, "case-01-allow-read.yaml", body)); !errors.Is(err, labspec.ErrInvalid) {
		t.Errorf("double: err = %v, want ErrInvalid", err)
	}
	verifier := strings.Replace(verifierScenario, "victim-fs: { calls_served: {} }",
		"victim-fs: { calls_served: {}, calls_refused: { fs.read: 0 } }", 1)
	if _, err := loadVerifier(t, verifier); !errors.Is(err, labspec.ErrInvalid) {
		t.Errorf("verifier: err = %v, want ErrInvalid", err)
	}
}
