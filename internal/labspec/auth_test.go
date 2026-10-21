package labspec_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

const authScenario = `schema_version: 1
id: verify-01-drift
title: an authenticated server is probed with the run's token
profile: [verifier]
auth_double: honest
verifier:
  - probe: { server: victim-vault, credential: true }
  - probe: { server: victim-vault-sessionauth, credential: true }
expect:
  verifier:
    1: { exit_code: 0 }
    2: { exit_code: 1 }
  effects:
    victim-vault:
      calls_served: {}
      auth_include: [{ credential: run }, { credential: other, status: 401 }]
    victim-vault-sessionauth:
      calls_served: {}
      auth_include: [{ credential: none, session: known, status: 200 }]
`

func TestAProbeWithTheRunsTokenAndTheAuthItLeftLoad(t *testing.T) {
	scenario, err := loadVerifier(t, authScenario)
	if err != nil {
		t.Fatalf("LoadScenario: %v", err)
	}
	if scenario.AuthDouble != "honest" || !scenario.Verifier[0].Probe.Credential {
		t.Errorf("auth_double %q, step 1 credential %t", scenario.AuthDouble, scenario.Verifier[0].Probe.Credential)
	}
	got := scenario.Expect.Effects["victim-vault-sessionauth"].AuthInclude
	want := labspec.AuthExpectation{Credential: "none", Session: "known", Status: 200}
	if len(got) != 1 || got[0] != want {
		t.Errorf("auth_include = %+v, want [%+v]", got, want)
	}
}

func TestAnAuthExpectationNothingCouldMeetIsRefused(t *testing.T) {
	for name, replace := range map[string][]string{
		"a persona that leaves its directory": {"auth_double: honest", "auth_double: ../honest"},
		"a persona with a suffix":             {"auth_double: honest", "auth_double: honest.json"},
		"an unknown credential":               {"{ credential: run }", "{ credential: token }"},
		"an unknown session":                  {"session: known", "session: stolen"},
		"a status no server answers":          {"status: 401", "status: 42"},
		"no credential named":                 {"{ credential: run }", "{ session: none }"},
		"auth on a victim that keeps none":    {"victim-vault-sessionauth", "victim-crm"},
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.NewReplacer(replace...).Replace(authScenario)
			if body == authScenario {
				t.Fatal("the replacement changed nothing")
			}
			if _, err := loadVerifier(t, body); !errors.Is(err, labspec.ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

// The double boots in the verifier's profile alone, so a persona named by a
// run through the enforcer would be served to nobody.
func TestAPersonaOutsideAVerifierScenarioIsRefused(t *testing.T) {
	body := strings.Replace(heldScenario, "expect:\n", "auth_double: honest\nexpect:\n", 1)
	if _, err := labspec.LoadScenario(writeFile(t, "case-01-allow-read.yaml", body)); !errors.Is(err, labspec.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}
