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
