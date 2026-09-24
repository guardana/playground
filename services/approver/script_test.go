package main

import (
	"errors"
	"testing"
	"time"
)

func TestParseScriptReadsEveryAnswer(t *testing.T) {
	s, err := parseScript([]byte(`schema_version: 1
rules:
  - match: {action: refund, resource: payment pay-9}
    answer: approve
    approver_id: lab-approver
    reason: scripted yes
    delay: 1500ms
  - match: {effect_class: EFFECT_CLASS_DELETE}
    answer: reject
    approver_id: lab-approver
  - match: {principal: user-3, agent: agent-9}
    answer: leave
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Rules) != 3 {
		t.Fatalf("rules = %d, want 3", len(s.Rules))
	}
	first := s.Rules[0]
	if first.Answer != answerApprove || first.ApproverID != "lab-approver" || first.Reason != "scripted yes" || first.after != 1500*time.Millisecond {
		t.Errorf("rule 1 read as %+v", first)
	}
	if s.Rules[1].Answer != answerReject || s.Rules[1].after != 0 || s.Rules[2].Answer != answerLeave {
		t.Errorf("rules 2 and 3 read as %+v, %+v", s.Rules[1], s.Rules[2])
	}
}

// A misspelled key that loaded would be a rule the scenario thinks it wrote
// and nobody did.
func TestParseScriptRefusesWhatItWouldMisread(t *testing.T) {
	cases := map[string]string{
		"an unknown top-level key":      "schema_version: 1\nrulez: []\n",
		"an unknown rule key":           "schema_version: 1\nrules:\n  - {answer: leave, when: later}\n",
		"an unknown match key":          "schema_version: 1\nrules:\n  - {match: {upstream: payments}, answer: leave}\n",
		"another schema version":        "schema_version: 2\nrules: []\n",
		"no schema version":             "rules: []\n",
		"an unknown answer":             "schema_version: 1\nrules:\n  - {answer: maybe}\n",
		"no answer":                     "schema_version: 1\nrules:\n  - {match: {action: refund}}\n",
		"approve naming nobody":         "schema_version: 1\nrules:\n  - {answer: approve}\n",
		"reject naming nobody":          "schema_version: 1\nrules:\n  - {answer: reject, reason: no}\n",
		"leave with an approver":        "schema_version: 1\nrules:\n  - {answer: leave, approver_id: a}\n",
		"leave with a reason":           "schema_version: 1\nrules:\n  - {answer: leave, reason: r}\n",
		"leave with a delay":            "schema_version: 1\nrules:\n  - {answer: leave, delay: 1s}\n",
		"a delay it cannot read":        "schema_version: 1\nrules:\n  - {answer: approve, approver_id: a, delay: soon}\n",
		"a negative delay":              "schema_version: 1\nrules:\n  - {answer: approve, approver_id: a, delay: -1s}\n",
		"not YAML":                      "schema_version: [1\n",
		"an effect class never printed": "schema_version: 1\nrules:\n  - {match: {effect_class: DELETE}, answer: leave}\n",
		"unreadable with a field":       "schema_version: 1\nrules:\n  - {match: {unreadable: true, action: refund}, answer: leave}\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseScript([]byte(body)); !errors.Is(err, errInvalidScript) {
				t.Errorf("err = %v, want errInvalidScript", err)
			}
		})
	}
}

func TestRuleForTakesTheFirstMatchAndNamesIt(t *testing.T) {
	s, err := parseScript([]byte(`schema_version: 1
rules:
  - match: {action: refund, resource: payment pay-1}
    answer: reject
    approver_id: a
  - match: {action: refund}
    answer: approve
    approver_id: a
  - match: {}
    answer: leave
`))
	if err != nil {
		t.Fatal(err)
	}
	refund := entry{id: "A", action: "refund", resource: "payment pay-9", readable: true}
	if r, n := s.ruleFor(refund); r == nil || r.Answer != answerApprove || n != 2 {
		t.Errorf("refund of pay-9: rule %d %+v, want rule 2 approving", n, r)
	}
	other := entry{id: "B", action: "delete_customer", readable: true}
	if r, n := s.ruleFor(other); r == nil || r.Answer != answerLeave || n != 3 {
		t.Errorf("delete_customer: rule %d %+v, want rule 3 leaving", n, r)
	}
}

func TestRuleForMatchesNothingWhenNoRuleCovers(t *testing.T) {
	s, err := parseScript([]byte("schema_version: 1\nrules:\n  - {match: {action: refund}, answer: leave}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if r, n := s.ruleFor(entry{id: "A", action: "refun", readable: true}); r != nil || n != 0 {
		t.Errorf("a near miss matched rule %d", n)
	}
}
