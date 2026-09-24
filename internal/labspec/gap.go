package labspec

import (
	"fmt"
	"path/filepath"
	"strings"
)

// GapClass is the scenario directory named gaps live in. They run on every
// build and are reported as the known-gap suite: a gap that still holds
// passes, and one whose behaviour changed fails like anything else that changed.
const GapClass = "gaps"

// Gap names behaviour the system under test does not have yet. The scenario's
// expectations are what the system's documentation says it does today; Wanted
// is what it should do, printed beside every run and never graded.
type Gap struct {
	Wanted map[int]DecisionExpectation `json:"wanted"`
	Why    string                      `json:"why"`
}

// Class is the directory a scenario file sits in.
func Class(path string) string { return filepath.Base(filepath.Dir(path)) }

func validateGap(s Scenario, class string) error {
	switch {
	case class == GapClass && s.Gap == nil:
		return fmt.Errorf("%w: a scenario under %s/ names no gap", ErrInvalid, GapClass)
	case class != GapClass && s.Gap != nil:
		return fmt.Errorf("%w: gap is set and the scenario is not under %s/, so the gap would be reported as a catalogue scenario",
			ErrInvalid, GapClass)
	case s.Gap == nil:
		return nil
	case strings.TrimSpace(s.Gap.Why) == "":
		return fmt.Errorf("%w: gap.why is empty", ErrInvalid)
	case len(s.Gap.Wanted) == 0:
		return fmt.Errorf("%w: gap.wanted is empty, so the gap names nothing it wants", ErrInvalid)
	}
	for _, number := range sortedInts(s.Gap.Wanted) {
		field := fmt.Sprintf("gap.wanted[%d].verdict", number)
		if err := oneOf(field, s.Gap.Wanted[number].Verdict, Verdicts...); err != nil {
			return err
		}
		if _, graded := s.Expect.Decisions[number]; !graded {
			return fmt.Errorf("%w: gap.wanted names step %d, which expect.decisions does not grade", ErrInvalid, number)
		}
	}
	return nil
}
