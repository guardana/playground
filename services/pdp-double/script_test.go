package main

import (
	"errors"
	"testing"
)

func TestScriptRefusesWhatItCannotRead(t *testing.T) {
	for name, body := range map[string]string{
		"unknown top-level key": "schema_version: 1\nrules: []\ndefault: allow\n",
		"unknown match key":     "schema_version: 1\nrules:\n  - match: {tool: crm.refund}\n    answer: allow\n",
		"unknown rule key":      "schema_version: 1\nrules:\n  - match: {action: crm.refund}\n    answer: allow\n    times: 1\n",
		"unknown answer":        "schema_version: 1\nrules:\n  - match: {action: crm.refund}\n    answer: permit\n",
		"unscripted scripted":   "schema_version: 1\nrules:\n  - match: {action: crm.refund}\n    answer: unscripted\n",
		"no answer":             "schema_version: 1\nrules:\n  - match: {action: crm.refund}\n",
		"no schema version":     "rules: []\n",
		"another version":       "schema_version: 2\nrules: []\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseScript([]byte(body)); !errors.Is(err, errInvalidScript) {
				t.Fatalf("parseScript = %v, want errInvalidScript", err)
			}
		})
	}
}

func TestAScriptWithoutRulesAnswersEverythingUnscripted(t *testing.T) {
	sc, err := parseScript([]byte("schema_version: 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	q := question{Action: named{Name: "crm.refund"}, Resource: entity{Type: "order", ID: "ord-1"}}
	if got := sc.answerFor(q); got != "unscripted" {
		t.Fatalf("answerFor = %q, want unscripted", got)
	}
}

func TestTheFirstMatchingRuleAnswers(t *testing.T) {
	sc, err := parseScript([]byte(`schema_version: 1
rules:
  - match: {action: crm.refund}
    answer: timeout
  - match: {action: crm.refund, resource_id: ord-1}
    answer: allow
`))
	if err != nil {
		t.Fatal(err)
	}
	q := question{Action: named{Name: "crm.refund"}, Resource: entity{Type: "order", ID: "ord-1"}}
	if got := sc.answerFor(q); got != "timeout" {
		t.Fatalf("answerFor = %q, want timeout", got)
	}
}
