package main

import (
	"errors"
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

const (
	scriptSchemaVersion = 1
	maxScriptBytes      = 1 << 20
)

var errInvalidScript = errors.New("pdp-double: invalid script")

// answer is the behaviour applied to one question. Its name is the journal
// line's detail, so a scenario asserts it verbatim.
type answer string

const (
	answerAllow           answer = "allow"
	answerDeny            answer = "deny"
	answerAllowObligation answer = "allow_obligation"
	answerTimeout         answer = "timeout"
	answerStatus500       answer = "status_500"
	answerNoEcho          answer = "no_echo"
	answerMalformed       answer = "malformed"
	answerExtraMember     answer = "extra_member"
	// answerUnscripted is what a question no rule matched gets. It cannot be
	// scripted: it exists to be told apart from a deny someone wrote down.
	answerUnscripted answer = "unscripted"
)

func (a answer) scriptable() bool {
	switch a {
	case answerAllow, answerDeny, answerAllowObligation, answerTimeout,
		answerStatus500, answerNoEcho, answerMalformed, answerExtraMember:
		return true
	}
	return false
}

// match narrows a rule to the questions carrying these values. An empty field
// matches anything; a rule with no field set matches every question.
type match struct {
	Action       string `json:"action,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	SubjectType  string `json:"subject_type,omitempty"`
	SubjectID    string `json:"subject_id,omitempty"`
}

type rule struct {
	Match  match  `json:"match"`
	Answer answer `json:"answer"`
}

// script is the answers one scenario scripted, tried in order.
type script struct {
	SchemaVersion int    `json:"schema_version"`
	Rules         []rule `json:"rules"`
}

// question is the part of an AuthZEN evaluation request a rule can match on.
// Everything else control sends is read past.
type question struct {
	Subject  entity `json:"subject"`
	Action   named  `json:"action"`
	Resource entity `json:"resource"`
}

type entity struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type named struct {
	Name string `json:"name"`
}

func loadScript(path string) (*script, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxScriptBytes {
		return nil, fmt.Errorf("%s: %w: %d bytes, limit %d", path, errInvalidScript, info.Size(), maxScriptBytes)
	}
	body, err := os.ReadFile(path) // #nosec G304 -- the path is the script the scenario mounted.
	if err != nil {
		return nil, err
	}
	parsed, err := parseScript(body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return parsed, nil
}

// parseScript is strict: a misspelled key that loaded would be a rule the
// scenario thinks it wrote and nobody did.
func parseScript(body []byte) (*script, error) {
	var parsed script
	if err := yaml.UnmarshalStrict(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidScript, err)
	}
	if parsed.SchemaVersion != scriptSchemaVersion {
		return nil, fmt.Errorf("%w: schema_version is %d, want %d", errInvalidScript, parsed.SchemaVersion, scriptSchemaVersion)
	}
	for i, r := range parsed.Rules {
		if !r.Answer.scriptable() {
			return nil, fmt.Errorf("%w: rule %d answers %q, not a scriptable answer", errInvalidScript, i+1, r.Answer)
		}
	}
	return &parsed, nil
}

// answerFor is the first matching rule's answer, and answerUnscripted when no
// rule matches.
func (s *script) answerFor(q question) answer {
	for _, r := range s.Rules {
		if r.Match.covers(q) {
			return r.Answer
		}
	}
	return answerUnscripted
}

func (m match) covers(q question) bool {
	for _, pair := range [][2]string{
		{m.Action, q.Action.Name},
		{m.ResourceType, q.Resource.Type},
		{m.ResourceID, q.Resource.ID},
		{m.SubjectType, q.Subject.Type},
		{m.SubjectID, q.Subject.ID},
	} {
		if pair[0] != "" && pair[0] != pair[1] {
			return false
		}
	}
	return true
}
