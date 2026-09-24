package labspec_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

const (
	effectsLine = "    victim-mail: { calls_served: {} }\n"
	coreProfile = "profile: [core, enforcer]"
)

// withDouble adds a double's journal to heldScenario's effects and, when
// profile is set, the profile and script that bring that double up.
func withDouble(journal, profile, scriptKey string) string {
	body := strings.Replace(heldScenario, effectsLine,
		effectsLine+"    "+journal+": { calls_served: { approve: 1 } }\n", 1)
	if profile == "" {
		return body
	}
	body = strings.Replace(body, coreProfile, "profile: [core, enforcer, "+profile+"]", 1)
	return strings.Replace(body, policyLine, policyLine+"  "+scriptKey+": lab.yaml\n", 1)
}

// A double's journal is graded like a victim's when the run brings it up.
func TestADoublesJournalIsGradedWhenItsProfileBringsItUp(t *testing.T) {
	for journal, double := range map[string][2]string{
		"pdp-double": {"pdp", "pdp_script"},
		"approver":   {"approvals", "approver_script"},
	} {
		t.Run(journal, func(t *testing.T) {
			scenario, trajectory := loadPair(t, withDouble(journal, double[0], double[1]))
			if err := labspec.Validate(scenario, trajectory); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if got := scenario.Expect.Effects[journal].CallsServed["approve"]; got != 1 {
				t.Errorf("%s calls_served approve = %d, want 1", journal, got)
			}
		})
	}
}

// Without its profile the double never runs, so its journal would be absent
// and the entry would grade a service the scenario never started.
func TestADoublesJournalWithoutItsProfileIsRefused(t *testing.T) {
	for _, journal := range []string{"pdp-double", "approver", "collector"} {
		t.Run(journal, func(t *testing.T) {
			scenario, trajectory := loadPair(t, withDouble(journal, "", ""))
			if err := labspec.Validate(scenario, trajectory); !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

// The other double's profile does not bring this one up.
func TestADoublesJournalUnderTheOtherDoublesProfileIsRefused(t *testing.T) {
	scenario, trajectory := loadPair(t, withDouble("pdp-double", "approvals", "approver_script"))
	if err := labspec.Validate(scenario, trajectory); !errors.Is(err, labspec.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestAStepNamesTheDecisionPointItsDecisionConsulted(t *testing.T) {
	for want, step := range map[string]string{
		"none":                    "1: { verdict: REQUIRE_APPROVAL, pdp_instance: none,",
		"https://pdp-double:8443": "1: { verdict: REQUIRE_APPROVAL, pdp_instance: 'https://pdp-double:8443',",
	} {
		body := strings.Replace(heldScenario, "1: { verdict: REQUIRE_APPROVAL,", step, 1)
		scenario, err := labspec.LoadScenario(writeFile(t, "stub-01-allow-read.yaml", body))
		if err != nil {
			t.Fatalf("LoadScenario: %v", err)
		}
		if got := scenario.Expect.Decisions[1].PDPInstance; got != want {
			t.Errorf("pdp_instance = %q, want %q", got, want)
		}
	}
}

// pdp_instance is read from POLICY_DECIDED, so a step with no decision of its
// own to read cannot state it, and a value that is neither none nor an
// identifier is a typo that would never match.
func TestAPDPInstanceWithNothingToReadOrNoIdentifierIsRefused(t *testing.T) {
	for name, replace := range map[string][2]string{
		"on a step that opens none": {"2: { opens: none }", "2: { opens: none, pdp_instance: none }"},
		"on a resuming step":        {"3: { resumes: 1,", "3: { resumes: 1, pdp_instance: none,"},
		"a bare service name":       {"1: { verdict: REQUIRE_APPROVAL,", "1: { verdict: REQUIRE_APPROVAL, pdp_instance: pdp-double,"},
		"plain http":                {"1: { verdict: REQUIRE_APPROVAL,", "1: { verdict: REQUIRE_APPROVAL, pdp_instance: 'http://pdp-double:8443',"},
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.Replace(heldScenario, replace[0], replace[1], 1)
			if body == heldScenario {
				t.Fatal("the replacement matched nothing")
			}
			if _, err := labspec.LoadScenario(writeFile(t, "stub-01-allow-read.yaml", body)); !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

// A tolerated INDETERMINATE passes its step before pdp_instance is read, so a
// tolerated step that states one states something nobody grades.
func TestAToleratedStepStatesNoPDPInstance(t *testing.T) {
	tolerated := heldScenario + "tolerance: { allow_indeterminate_for_steps: [1] }\n"
	scenario, trajectory := loadPair(t, tolerated)
	if err := labspec.Validate(scenario, trajectory); err != nil {
		t.Fatalf("a tolerated step without pdp_instance was refused: %v", err)
	}
	stated := strings.Replace(tolerated, "1: { verdict: REQUIRE_APPROVAL,", "1: { verdict: REQUIRE_APPROVAL, pdp_instance: none,", 1)
	scenario, trajectory = loadPair(t, stated)
	if err := labspec.Validate(scenario, trajectory); !errors.Is(err, labspec.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}
