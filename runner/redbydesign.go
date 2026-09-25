package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/redbydesign"
)

// ranScenario is what one scenario of a run established.
type ranScenario struct {
	id      string
	outcome assertion.Outcome
}

// redByDesignList reads the list -red-by-design names, before anything runs.
// It judges a whole catalogue only: against one scenario every other listed id
// would read as a scenario that does not exist.
func redByDesignList(chosen settings) ([]redbydesign.Entry, error) {
	if chosen.redByDesign == "" {
		return nil, nil
	}
	if !chosen.all {
		return nil, errors.New("-red-by-design judges the whole catalogue; give it with -all")
	}
	listed, err := redbydesign.Read(chosen.redByDesign)
	if err != nil {
		return nil, fmt.Errorf("-red-by-design: %w", err)
	}
	return listed, nil
}

// executeRedByDesign runs every scenario and judges the run against the list.
func executeRedByDesign(ctx context.Context, subject lab, scenarios []string, listed []redbydesign.Entry, out io.Writer) error {
	return judgeRedByDesign(runEach(ctx, subject, scenarios, out), listed, out)
}

// judgeRedByDesign passes a run only when the scenarios that did not pass are
// exactly the listed ones, each failed on a record. A listed scenario that is
// indeterminate established nothing, so its finding was not seen either.
func judgeRedByDesign(ran []ranScenario, listed []redbydesign.Entry, out io.Writer) error {
	outcomes := map[string]assertion.Outcome{}
	for _, scenario := range ran {
		outcomes[scenario.id] = scenario.outcome
	}
	isListed := map[string]bool{}
	var problems []string
	for _, entry := range listed {
		isListed[entry.ID] = true
		outcome, found := outcomes[entry.ID]
		switch {
		case !found:
			problems = append(problems, entry.ID+" is listed red by design and is no scenario of this run")
		case outcome == assertion.Pass:
			problems = append(problems, entry.ID+" is listed red by design and passed; take it off the list if its finding is fixed")
		case outcome != assertion.Fail:
			problems = append(problems, fmt.Sprintf("%s is listed red by design and is %s, not red on a record", entry.ID, outcome))
		}
	}
	passed := 0
	for _, scenario := range ran {
		switch {
		case scenario.outcome == assertion.Pass:
			passed++
		case !isListed[scenario.id]:
			problems = append(problems, fmt.Sprintf("%s is %s and not listed red by design", scenario.id, scenario.outcome))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("red by design: %s", strings.Join(problems, "; "))
	}
	for _, entry := range listed {
		_, _ = fmt.Fprintf(out, "red by design  %s: %s\n", entry.ID, entry.Finding)
	}
	_, _ = fmt.Fprintf(out, "red by design: %d passed, %d red as listed, of %d scenarios\n", passed, len(listed), len(ran))
	return nil
}
