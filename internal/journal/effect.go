package journal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"unicode/utf8"
)

// Bounds on what one effect may carry. An effect is graded member for member,
// so a value is refused rather than shortened: a cut value would be a
// different value.
const (
	MaxEffectMembers = 16
	MaxMemberName    = 64
	MaxStringValue   = 256
	// MaxInteger is the largest magnitude every JSON reader holds exactly;
	// past it two different amounts can decode to one.
	MaxInteger = 1<<53 - 1
)

// ErrInvalidEffect reports an effect this package will neither write nor read.
var ErrInvalidEffect = errors.New("journal: invalid effect")

// Effect is what a served call changed, as the victim that changed it states
// it: a flat object of member names to values. It is written in the line that
// records the call, so the line is the record of the change.
type Effect map[string]Value

// Value is one member of an effect: a string, a boolean or an integer. The
// zero Value is none of them and is refused.
type Value struct{ v any }

// String is a string value.
func String(s string) Value { return Value{v: s} }

// Integer is an integer value; past ±MaxInteger it is refused on the way out.
func Integer(n int64) Value { return Value{v: n} }

// Bool is a boolean value.
func Bool(b bool) Value { return Value{v: b} }

// Validate refuses an effect with no member, too many, a name nobody can
// match, or a value outside the three kinds and their bounds.
func (e Effect) Validate() error {
	if len(e) == 0 {
		return fmt.Errorf("%w: no member", ErrInvalidEffect)
	}
	if len(e) > MaxEffectMembers {
		return fmt.Errorf("%w: %d members, limit %d", ErrInvalidEffect, len(e), MaxEffectMembers)
	}
	for name, value := range e {
		if name == "" || len(name) > MaxMemberName || !utf8.ValidString(name) {
			return fmt.Errorf("%w: member name %q is empty, longer than %d bytes or not UTF-8",
				ErrInvalidEffect, name, MaxMemberName)
		}
		if err := value.validate(); err != nil {
			return fmt.Errorf("member %q: %w", name, err)
		}
	}
	return nil
}

func (v Value) validate() error {
	switch value := v.v.(type) {
	case string:
		if len(value) > MaxStringValue || !utf8.ValidString(value) {
			return fmt.Errorf("%w: a string longer than %d bytes or not UTF-8", ErrInvalidEffect, MaxStringValue)
		}
	case int64:
		if value > MaxInteger || value < -MaxInteger {
			return fmt.Errorf("%w: integer %d is past ±%d", ErrInvalidEffect, value, int64(MaxInteger))
		}
	case bool:
	default:
		return fmt.Errorf("%w: no value", ErrInvalidEffect)
	}
	return nil
}

// MarshalJSON writes the value as the JSON scalar it is.
func (v Value) MarshalJSON() ([]byte, error) {
	if err := v.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(v.v)
}

// UnmarshalJSON reads the raw token, never a float64: 5000.0, 5e3 and an
// integer past MaxInteger are refused rather than rounded into a match.
// Scenario files load through this decoder too, so both sides of a comparison
// are read by one set of rules.
func (v *Value) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return fmt.Errorf("%w: no value", ErrInvalidEffect)
	}
	switch c := data[0]; {
	case c == '"':
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*v = String(s)
	case c == 't' || c == 'f':
		var b bool
		if err := json.Unmarshal(data, &b); err != nil {
			return err
		}
		*v = Bool(b)
	case c == '-' || (c >= '0' && c <= '9'):
		n, err := strconv.ParseInt(string(data), 10, 64)
		if err != nil {
			return fmt.Errorf("%w: %s is not an integer", ErrInvalidEffect, data)
		}
		*v = Integer(n)
	default:
		return fmt.Errorf("%w: %s is not a string, a boolean or an integer", ErrInvalidEffect, data)
	}
	return v.validate()
}

// String renders the value as it is written in a journal line.
func (v Value) String() string {
	line, err := v.MarshalJSON()
	if err != nil {
		return "<invalid>"
	}
	return string(line)
}

// Equal reports whether two values are the same kind and the same value.
func (v Value) Equal(other Value) bool { return v == other }

// Equal reports whether two effects hold the same members with equal values.
func (e Effect) Equal(other Effect) bool {
	if len(e) != len(other) {
		return false
	}
	for name, value := range e {
		theirs, held := other[name]
		if !held || !value.Equal(theirs) {
			return false
		}
	}
	return true
}

// String renders the effect as one JSON object, members in name order.
func (e Effect) String() string {
	line, err := json.Marshal(e)
	if err != nil {
		return "<invalid>"
	}
	return string(line)
}
