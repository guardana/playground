package labspec

import (
	"fmt"
	"regexp"
)

// HealthExpectation is what the enforcer's /healthz counts under pipeline
// once the replay is over. Every stated count is exact; a field left out is
// not asserted. Blocks names reason codes, and a code it leaves out is not
// asserted either.
type HealthExpectation struct {
	Blocks                   map[string]int `json:"blocks,omitempty"`
	ReadsUnrecorded          *int           `json:"reads_unrecorded,omitempty"`
	SinkFailuresBeforeEffect *int           `json:"sink_failures_before_effect,omitempty"`
}

// reasonCode is how the enforcer's registry spells a reason code.
var reasonCode = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// validateHealth refuses an expect.health that asserts nothing, one no
// enforcer answers, and a count no counter can hold. A block count below one
// is refused because an absent reason reads as zero blocks, so a misspelled
// code stated at zero would pass on every run.
func (s Scenario) validateHealth() error {
	health := s.Expect.Health
	if health == nil {
		return nil
	}
	if !s.UsesEnforcer() {
		return fmt.Errorf("%w: expect.health is set and no enforcer runs to answer /healthz", ErrInvalid)
	}
	if health.Blocks == nil && health.ReadsUnrecorded == nil && health.SinkFailuresBeforeEffect == nil {
		return fmt.Errorf("%w: expect.health states no count", ErrInvalid)
	}
	if err := health.validateBlocks(); err != nil {
		return err
	}
	for _, field := range []struct {
		name  string
		count *int
	}{
		{"reads_unrecorded", health.ReadsUnrecorded},
		{"sink_failures_before_effect", health.SinkFailuresBeforeEffect},
	} {
		if field.count != nil && *field.count < 0 {
			return fmt.Errorf("%w: expect.health.%s is %d, a count below zero", ErrInvalid, field.name, *field.count)
		}
	}
	return nil
}

func (h HealthExpectation) validateBlocks() error {
	if h.Blocks != nil && len(h.Blocks) == 0 {
		return fmt.Errorf("%w: expect.health.blocks names no reason code", ErrInvalid)
	}
	for _, code := range sortedKeys(h.Blocks) {
		if !reasonCode.MatchString(code) || h.Blocks[code] < 1 {
			return fmt.Errorf("%w: expect.health.blocks has %q: %d, want a reason code as the registry spells it and a count of at least 1",
				ErrInvalid, code, h.Blocks[code])
		}
	}
	return nil
}
