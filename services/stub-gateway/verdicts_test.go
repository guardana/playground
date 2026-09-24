package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const wellFormed = `schema_version: 1
project_id: playground
tenant_id: tenant_a
policy_bundle_digest: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
steps:
  - verdict: ALLOW
    reason_codes: [RULE_ALLOW]
  - verdict: ALLOW_WITH_OBLIGATIONS
    reason_codes: [OBLIGATIONS_ATTACHED]
    obligations: [{ type: label_sensitive }]
  - verdict: DENY
    reason_codes: [TOXIC_FLOW_SENSITIVE_TO_EXTERNAL]
`

func writeFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "verdicts.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadVerdictsReadsTheDeclaredSteps(t *testing.T) {
	declared, err := loadVerdicts(writeFile(t, wellFormed))
	if err != nil {
		t.Fatalf("loadVerdicts: %v", err)
	}
	if declared.ProjectID != "playground" || declared.TenantID != "tenant_a" {
		t.Errorf("project %q tenant %q, want playground and tenant_a", declared.ProjectID, declared.TenantID)
	}
	second := declared.at(2)
	if second.verdict != "VERDICT_ALLOW_WITH_OBLIGATIONS" {
		t.Errorf("step 2 verdict is %q", second.verdict)
	}
	if len(second.obligations) != 1 || second.obligations[0].Type != "label_sensitive" {
		t.Errorf("step 2 obligations are %+v", second.obligations)
	}
	if !second.forwards() {
		t.Error("ALLOW_WITH_OBLIGATIONS does not forward")
	}
	if declared.at(3).forwards() {
		t.Error("DENY forwards")
	}
}

// Running out of declared verdicts is the case the lab depends on: a trajectory
// longer than its verdict file must not pass, and an allow is the one answer
// that would let it.
func TestAStepBeyondTheDeclaredOnesIsIndeterminate(t *testing.T) {
	declared, err := loadVerdicts(writeFile(t, wellFormed))
	if err != nil {
		t.Fatalf("loadVerdicts: %v", err)
	}
	for _, step := range []int{4, 0, -1} {
		answer := declared.at(step)
		if answer.verdict != "VERDICT_INDETERMINATE" {
			t.Errorf("step %d: verdict is %q, want VERDICT_INDETERMINATE", step, answer.verdict)
		}
		if len(answer.reasonCodes) != 1 || answer.reasonCodes[0] != "STUB_NO_DECLARED_VERDICT" {
			t.Errorf("step %d: reason codes are %v", step, answer.reasonCodes)
		}
		if answer.forwards() {
			t.Errorf("step %d forwards", step)
		}
	}
}

func TestLoadVerdictsRefusesAKeyItHasNoFieldFor(t *testing.T) {
	body := wellFormed + "  - verdict: ALLOW\n    reason_code: [RULE_ALLOW]\n"
	if _, err := loadVerdicts(writeFile(t, body)); err == nil {
		t.Fatal("a misspelled key loaded")
	}
}

func TestLoadVerdictsRefusesAVerdictOutsideTheRegistry(t *testing.T) {
	body := "schema_version: 1\nproject_id: p\ntenant_id: t\npolicy_bundle_digest: sha256:x\nsteps:\n  - verdict: PROBABLY\n"
	_, err := loadVerdicts(writeFile(t, body))
	if !errors.Is(err, errInvalidVerdicts) {
		t.Fatalf("error is %v, want errInvalidVerdicts", err)
	}
}

func TestLoadVerdictsRefusesAnotherSchemaVersion(t *testing.T) {
	body := "schema_version: 2\nproject_id: p\ntenant_id: t\npolicy_bundle_digest: sha256:x\nsteps: []\n"
	if _, err := loadVerdicts(writeFile(t, body)); !errors.Is(err, errInvalidVerdicts) {
		t.Fatalf("error is %v, want errInvalidVerdicts", err)
	}
}

// The reason codes come from the enforcement plane's registry, which moves
// without this file. A stub that refused a code the real plane would emit would
// constrain the lab to the codes it happens to know.
func TestLoadVerdictsAcceptsAReasonCodeItDoesNotKnow(t *testing.T) {
	body := "schema_version: 1\nproject_id: p\ntenant_id: t\npolicy_bundle_digest: sha256:x\n" +
		"steps:\n  - verdict: DENY\n    reason_codes: [SOME_LATER_RULE]\n"
	declared, err := loadVerdicts(writeFile(t, body))
	if err != nil {
		t.Fatalf("loadVerdicts: %v", err)
	}
	if got := declared.at(1).reasonCodes; len(got) != 1 || got[0] != "SOME_LATER_RULE" {
		t.Errorf("reason codes are %v", got)
	}
}

// An empty file of declared verdicts is the honest default for a lab brought up
// by hand: every call is answered INDETERMINATE and recorded as such.
func TestNoDeclaredStepsIsLoadable(t *testing.T) {
	body := "schema_version: 1\nproject_id: p\ntenant_id: t\npolicy_bundle_digest: sha256:x\nsteps: []\n"
	declared, err := loadVerdicts(writeFile(t, body))
	if err != nil {
		t.Fatalf("loadVerdicts: %v", err)
	}
	if declared.at(1).verdict != "VERDICT_INDETERMINATE" {
		t.Errorf("verdict is %q", declared.at(1).verdict)
	}
}

// A scenario that declares INDETERMINATE and one whose verdict file simply ran
// out used to produce the same answer, byte for byte, so deleting the
// declaration left the run green. They have to differ, or a scenario can pass
// on silence: this is the one property that stops the stub from grading a file
// nobody wrote.
func TestSilenceIsDistinguishableFromADeclaredIndeterminate(t *testing.T) {
	declared := &verdicts{Steps: []declaredStep{
		{Verdict: "INDETERMINATE", ReasonCodes: []string{"POLICY_UNAVAILABLE"}},
	}}

	stated := declared.at(1)
	silent := declared.at(2)

	if stated.verdict != silent.verdict {
		t.Fatalf("the two answers already differ by verdict (%q and %q); this test is checking the wrong thing",
			stated.verdict, silent.verdict)
	}
	if slices.Equal(stated.reasonCodes, silent.reasonCodes) {
		t.Errorf("a declared INDETERMINATE and an undeclared step both answer %v; "+
			"deleting the declaration would leave a scenario green", stated.reasonCodes)
	}
	if !slices.Contains(silent.reasonCodes, NoDeclaredVerdict) {
		t.Errorf("an undeclared step answers %v, want it to carry %s", silent.reasonCodes, NoDeclaredVerdict)
	}
	if stated.forwards() || silent.forwards() {
		t.Error("an INDETERMINATE answer forwarded the call")
	}
}

// REQUIRE_APPROVAL is a verdict this stub cannot account for. Nobody here can be
// asked, so a call declaring it would be recorded as POLICY_DECIDED then
// ACTION_BLOCKED with no APPROVAL_REQUESTED between them: a trail saying an
// approval was required and never requested. ValidateChain accepts that shape
// and the decisions check compares only the verdict string, so a scenario would
// pass on a record that contradicts itself. The refusal happens at load, while
// the file that caused it is still in front of someone.
func TestLoadVerdictsRefusesAVerdictItCannotAccountFor(t *testing.T) {
	body := "schema_version: 1\nproject_id: p\ntenant_id: t\npolicy_bundle_digest: sha256:x\n" +
		"steps:\n  - verdict: ALLOW\n  - verdict: REQUIRE_APPROVAL\n"

	_, err := loadVerdicts(writeFile(t, body))
	if !errors.Is(err, errInvalidVerdicts) {
		t.Fatalf("error is %v, want errInvalidVerdicts", err)
	}
	for _, want := range []string{"steps[2]", "REQUIRE_APPROVAL", "approval"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q: %v", want, err)
		}
	}
}

// The four the stub can write a truthful trail for still load, INDETERMINATE
// included: refusing one verdict must not narrow the file to the allows.
func TestLoadVerdictsAcceptsEveryVerdictItCanAccountFor(t *testing.T) {
	body := "schema_version: 1\nproject_id: p\ntenant_id: t\npolicy_bundle_digest: sha256:x\nsteps:\n" +
		"  - verdict: ALLOW\n  - verdict: ALLOW_WITH_OBLIGATIONS\n  - verdict: DENY\n  - verdict: INDETERMINATE\n"

	declared, err := loadVerdicts(writeFile(t, body))
	if err != nil {
		t.Fatalf("loadVerdicts: %v", err)
	}
	if got := declared.at(4).verdict; got != "VERDICT_INDETERMINATE" {
		t.Errorf("step 4 verdict is %q", got)
	}
}
