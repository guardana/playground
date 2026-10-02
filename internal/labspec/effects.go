package labspec

import (
	"fmt"
	"slices"
	"strings"
)

// The journals the two doubles write, named as expect.effects names them.
const (
	PDPDoubleJournal = "pdp-double"
	ApproverJournal  = "approver"
)

// doubleProfile is the profile that brings up the double writing journal, and
// false for a name that is no double's.
func doubleProfile(journal string) (string, bool) {
	switch journal {
	case PDPDoubleJournal:
		return "pdp", true
	case ApproverJournal:
		return "approvals", true
	}
	return "", false
}

// validateEffects holds every victim the trajectory calls to an account of what
// it served, and lets a double's journal be graded the same way when the run
// brings that double up. A double's entry is optional: the agent never calls
// it, so leaving it out leaves no call unaccounted for.
func validateEffects(s Scenario, t Trajectory) error {
	for _, server := range servers(t) {
		if _, stated := s.Expect.Effects[server]; !stated {
			return fmt.Errorf("%w: the trajectory calls %s and expect.effects does not say what it served",
				ErrInvalid, server)
		}
	}
	for _, server := range sortedKeys(s.Expect.Effects) {
		if slices.Contains(servers(t), server) {
			continue
		}
		profile, double := doubleProfile(server)
		switch {
		case !double:
			return fmt.Errorf("%w: expect.effects names %s and the trajectory never calls it",
				ErrInvalid, server)
		case !slices.Contains(s.Profile, profile):
			return fmt.Errorf("%w: expect.effects names %s and profile %s, which brings it up, is not in the profile",
				ErrInvalid, server, profile)
		}
	}
	return nil
}

// validateEffectCounts refuses a calls_refused entry that states nothing: a
// count below one says no more than leaving the tool out, or counts lines no
// journal can hold, and a tool with no name matches no line.
func (s Scenario) validateEffectCounts() error {
	for _, journal := range sortedKeys(s.Expect.Effects) {
		refused := s.Expect.Effects[journal].CallsRefused
		for _, tool := range sortedKeys(refused) {
			if strings.TrimSpace(tool) == "" {
				return fmt.Errorf("%w: expect.effects.%s.calls_refused names a tool with no name", ErrInvalid, journal)
			}
			if refused[tool] < 1 {
				return fmt.Errorf("%w: expect.effects.%s.calls_refused.%s is %d, want at least 1 or the tool left out",
					ErrInvalid, journal, tool, refused[tool])
			}
		}
	}
	return nil
}

// validateCommitted refuses a committed list that can never pass. Every served
// line of a named tool has to carry an effect, so the list is exactly as long
// as calls_served for that tool; a double writes no effect, so it states none.
func (s Scenario) validateCommitted() error {
	for _, journal := range sortedKeys(s.Expect.Effects) {
		expect := s.Expect.Effects[journal]
		if _, double := doubleProfile(journal); double && len(expect.Committed) > 0 {
			return fmt.Errorf("%w: expect.effects.%s.committed is stated and a double records no effect", ErrInvalid, journal)
		}
		for _, tool := range sortedKeys(expect.Committed) {
			field := fmt.Sprintf("expect.effects.%s.committed.%s", journal, tool)
			list := expect.Committed[tool]
			switch {
			case strings.TrimSpace(tool) == "":
				return fmt.Errorf("%w: expect.effects.%s.committed names a tool with no name", ErrInvalid, journal)
			case len(list) == 0:
				return fmt.Errorf("%w: %s is empty, want one effect per served call or the tool left out", ErrInvalid, field)
			case len(list) != expect.CallsServed[tool]:
				return fmt.Errorf("%w: %s lists %d effect(s) and calls_served.%s is %d; every served call of a committing tool records one",
					ErrInvalid, field, len(list), tool, expect.CallsServed[tool])
			}
			for i, effect := range list {
				if err := effect.Validate(); err != nil {
					return fmt.Errorf("%w: %s[%d]: %w", ErrInvalid, field, i, err)
				}
			}
		}
	}
	return nil
}
