package labspec_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

func withHealth(entry string) string {
	return strings.Replace(goodScenario, "  evidence:\n", "  health: "+entry+"\n  evidence:\n", 1)
}

func TestHealthIsReadWithExactCounts(t *testing.T) {
	s, tr := loadPair(t, withHealth(
		"{ blocks: { EVIDENCE_UNAVAILABLE: 2 }, reads_unrecorded: 0, sink_failures_before_effect: 2 }"))
	if err := labspec.Validate(s, tr); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	health := s.Expect.Health
	if health == nil {
		t.Fatal("expect.health was not read")
	}
	if got := health.Blocks["EVIDENCE_UNAVAILABLE"]; got != 2 || len(health.Blocks) != 1 {
		t.Errorf("blocks = %v, want EVIDENCE_UNAVAILABLE=2", health.Blocks)
	}
	if health.ReadsUnrecorded == nil || *health.ReadsUnrecorded != 0 {
		t.Errorf("reads_unrecorded = %v, want a stated 0", health.ReadsUnrecorded)
	}
	if health.SinkFailuresBeforeEffect == nil || *health.SinkFailuresBeforeEffect != 2 {
		t.Errorf("sink_failures_before_effect = %v, want 2", health.SinkFailuresBeforeEffect)
	}
}

// A field left out is not asserted, which is different from asserting zero.
func TestAHealthFieldLeftOutIsNotAsserted(t *testing.T) {
	s, _ := loadPair(t, withHealth("{ reads_unrecorded: 1 }"))
	if s.Expect.Health.SinkFailuresBeforeEffect != nil || s.Expect.Health.Blocks != nil {
		t.Errorf("health = %+v, want only reads_unrecorded stated", *s.Expect.Health)
	}
}

func TestHealthRefusesWhatStatesNothingOrCannotBeCounted(t *testing.T) {
	for name, entry := range map[string]string{
		"empty":                     "{}",
		"empty blocks":              "{ blocks: {} }",
		"a block count of zero":     "{ blocks: { EVIDENCE_UNAVAILABLE: 0 } }",
		"a negative block count":    "{ blocks: { EVIDENCE_UNAVAILABLE: -1 } }",
		"a reason not so spelled":   "{ blocks: { evidence_unavailable: 1 } }",
		"a negative unrecorded":     "{ reads_unrecorded: -1 }",
		"a negative sink failure":   "{ sink_failures_before_effect: -2 }",
		"a misspelled field":        "{ reads_unrecordd: 1 }",
		"a field the lab reads not": "{ executed: 1 }",
		"a misspelled second field": "{ reads_unrecorded: 1, sink_failures_before_efect: 2 }",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := labspec.LoadScenario(writeFile(t, "case-01-allow-read.yaml", withHealth(entry)))
			if !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

// Only the enforcer answers /healthz; a verifier scenario boots none.
func TestHealthIsRefusedWhereNoEnforcerDecides(t *testing.T) {
	body := strings.Replace(verifierScenario, "  effects:\n", "  health: { reads_unrecorded: 0 }\n  effects:\n", 1)
	_, err := loadVerifier(t, body)
	if !errors.Is(err, labspec.ErrInvalid) || !strings.Contains(err.Error(), "expect.health") {
		t.Fatalf("err = %v, want ErrInvalid naming expect.health", err)
	}
}
