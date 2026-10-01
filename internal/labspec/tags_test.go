package labspec_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

func TestAStepStatesTheTagsOfItsProposal(t *testing.T) {
	body := strings.Replace(heldScenario, heldStepOne,
		`1: { verdict: REQUIRE_APPROVAL, proposed_tags_include: ["flow.v1.untrusted=true", "flow.v1.max_read=PUBLIC"] }`, 1)
	if body == heldScenario {
		t.Fatal("the replacement changed nothing")
	}
	scenario, trajectory := loadPair(t, body)
	if err := labspec.Validate(scenario, trajectory); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got := scenario.Expect.Decisions[1].ProposedTagsInclude; len(got) != 2 || got[1] != "flow.v1.max_read=PUBLIC" {
		t.Errorf("step 1 tags = %v", got)
	}
}

func TestProposedTagsNoProposalOfTheStepCarriesAreRefused(t *testing.T) {
	for name, replace := range map[string][2]string{
		"an empty tag":           {heldStepOne, `1: { verdict: REQUIRE_APPROVAL, proposed_tags_include: [""] }`},
		"a step that opens none": {"2: { opens: none }", `2: { opens: none, proposed_tags_include: ["flow.v1.untrusted=true"] }`},
		"a resuming step":        {heldStepThree, `3: { resumes: 1, trail: [ACTION_PROPOSED], proposed_tags_include: ["flow.v1.untrusted=true"] }`},
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.Replace(heldScenario, replace[0], replace[1], 1)
			if body == heldScenario {
				t.Fatal("the replacement changed nothing")
			}
			if _, err := labspec.LoadScenario(writeFile(t, "case-01-allow-read.yaml", body)); !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}
