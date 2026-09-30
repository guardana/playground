package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"sigs.k8s.io/yaml"
)

const (
	scriptSchemaVersion = 1
	maxScriptBytes      = 1 << 20
)

var errInvalidScript = errors.New("approver: invalid script")

// answer is what a rule does with a waiting approval. Its name is the journal
// line's tool, so a scenario asserts it verbatim.
type answer string

const (
	answerApprove answer = "approve"
	answerReject  answer = "reject"
	// answerLeave runs nothing, so the approval expires unless someone else
	// answers it. It is also what an approval no rule matches gets.
	answerLeave answer = "leave"
)

// state is the approval state the command writes for this answer.
func (a answer) state() string {
	if a == answerReject {
		return stateRejected
	}
	return stateApproved
}

// match narrows a rule to approvals whose listing shows these values. An empty
// field matches anything. Only what `approvals list` prints can be matched,
// and the upstream it prints is not matched yet. Unreadable matches only the
// records the listing shows no readable fields for, which no other rule
// matches.
type match struct {
	Action      string `json:"action,omitempty"`
	Resource    string `json:"resource,omitempty"`
	EffectClass string `json:"effect_class,omitempty"`
	Principal   string `json:"principal,omitempty"`
	Agent       string `json:"agent,omitempty"`
	Unreadable  bool   `json:"unreadable,omitempty"`
}

type rule struct {
	Match      match  `json:"match"`
	Answer     answer `json:"answer"`
	ApproverID string `json:"approver_id,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Delay      string `json:"delay,omitempty"`
	// after is Delay, read.
	after time.Duration
}

// script is the answers one scenario scripted, tried in order.
type script struct {
	SchemaVersion int    `json:"schema_version"`
	Rules         []rule `json:"rules"`
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
	for i := range parsed.Rules {
		if err := parsed.Rules[i].check(); err != nil {
			return nil, fmt.Errorf("%w: rule %d: %w", errInvalidScript, i+1, err)
		}
	}
	return &parsed, nil
}

// check refuses a rule that would do something other than what it reads as.
// The approver id and the reason are passed to the command as written, even
// ones it will refuse: that refusal is the enforcer's to make and to journal.
func (r *rule) check() error {
	if err := r.Match.check(); err != nil {
		return err
	}
	switch r.Answer {
	case answerApprove, answerReject:
		if r.ApproverID == "" {
			return fmt.Errorf("%s names no approver_id, and the command refuses an answer nobody claims", r.Answer)
		}
	case answerLeave:
		if r.ApproverID != "" || r.Reason != "" || r.Delay != "" {
			return errors.New("leave runs nothing, so it takes no approver_id, reason or delay")
		}
		return nil
	default:
		return fmt.Errorf("answer %q is not approve, reject or leave", r.Answer)
	}
	if r.Delay == "" {
		return nil
	}
	after, err := time.ParseDuration(r.Delay)
	if err != nil || after < 0 {
		return fmt.Errorf("delay %q is not a non-negative duration", r.Delay)
	}
	r.after = after
	return nil
}

// check refuses a value the listing never prints, which would leave every
// approval unmatched while reading as a rule.
func (m match) check() error {
	if m.Unreadable && m != (match{Unreadable: true}) {
		return errors.New("an unreadable record shows no fields, so unreadable takes no other match key")
	}
	if m.EffectClass != "" && !knownEffectClass(m.EffectClass) {
		return fmt.Errorf("effect_class %q is not a class the listing prints", m.EffectClass)
	}
	return nil
}

// knownEffectClass is every effect class the pinned command can print.
func knownEffectClass(v string) bool {
	switch v {
	case "EFFECT_CLASS_UNSPECIFIED", "EFFECT_CLASS_READ", "EFFECT_CLASS_WRITE", "EFFECT_CLASS_DELETE",
		"EFFECT_CLASS_EXECUTE", "EFFECT_CLASS_COMMUNICATE", "EFFECT_CLASS_TRANSACT",
		"EFFECT_CLASS_IDENTITY_OR_ACCESS", "EFFECT_CLASS_CONFIGURE", "EFFECT_CLASS_SPAWN_OR_DELEGATE":
		return true
	}
	return false
}

// ruleFor is the first rule matching e and its number counted from one, or nil
// and zero when no rule matches.
func (s *script) ruleFor(e entry) (*rule, int) {
	for i := range s.Rules {
		if s.Rules[i].Match.covers(e) {
			return &s.Rules[i], i + 1
		}
	}
	return nil, 0
}

func (m match) covers(e entry) bool {
	if m.Unreadable || !e.readable {
		return m.Unreadable && !e.readable
	}
	for _, pair := range [][2]string{
		{m.Action, e.action},
		{m.Resource, e.resource},
		{m.EffectClass, e.effectClass},
		{m.Principal, e.principal},
		{m.Agent, e.agent},
	} {
		if pair[0] != "" && pair[0] != pair[1] {
			return false
		}
	}
	return true
}
