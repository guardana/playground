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

// ranScenario is what one scenario of a run established: its outcome and
// every result it was graded on.
type ranScenario struct {
	id      string
	outcome assertion.Outcome
	results []ranCheck
}

// ranCheck is one result a scenario was graded on.
type ranCheck struct {
	id      string
	outcome assertion.Outcome
}

// ranChecks keeps the check id and outcome of every result in a report.
func ranChecks(results []assertion.Result) []ranCheck {
	ran := make([]ranCheck, 0, len(results))
	for _, result := range results {
		ran = append(ran, ranCheck{id: result.Check, outcome: result.Outcome})
	}
	return ran
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

// judgeRedByDesign passes a run only when every unlisted scenario passed and
// every listed one failed on exactly its named checks, every other result
// passing.
func judgeRedByDesign(ran []ranScenario, listed []redbydesign.Entry, out io.Writer) error {
	byID := map[string]ranScenario{}
	for _, scenario := range ran {
		byID[scenario.id] = scenario
	}
	isListed := map[string]bool{}
	var problems []string
	for _, entry := range listed {
		isListed[entry.ID] = true
		scenario, found := byID[entry.ID]
		if !found {
			problems = append(problems, entry.ID+" is listed red by design and is no scenario of this run")
			continue
		}
		problems = append(problems, judgeListed(entry, scenario)...)
	}
	passed := 0
	for _, scenario := range ran {
		switch {
		case isListed[scenario.id]:
		case scenario.outcome == assertion.Pass:
			passed++
		default:
			problems = append(problems, fmt.Sprintf("%s is %s and not listed red by design", scenario.id, scenario.outcome))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("red by design: %s", strings.Join(problems, "; "))
	}
	for _, entry := range listed {
		_, _ = fmt.Fprintf(out, "red by design  %s on %s: %s\n", entry.ID, strings.Join(entry.Checks, ","), entry.Finding)
	}
	_, _ = fmt.Fprintf(out, "red by design: %d passed, %d red as listed, of %d scenarios\n", passed, len(listed), len(ran))
	return nil
}

// judgeListed reads every result of a listed scenario. The scenario's outcome
// alone is not enough: Fail outranks Indeterminate, so a named fail would hide
// a check that established nothing, or another check failing.
func judgeListed(entry redbydesign.Entry, scenario ranScenario) []string {
	named := map[string]bool{}
	for _, check := range entry.Checks {
		named[check] = false
	}
	var problems []string
	for _, result := range scenario.results {
		_, isNamed := named[result.id]
		switch {
		case isNamed && result.outcome != assertion.Fail:
			problems = append(problems, fmt.Sprintf(
				"%s is listed red by design on %s, and that check is %s; take it off the list if its finding is fixed",
				entry.ID, result.id, result.outcome))
		case !isNamed && result.outcome != assertion.Pass:
			problems = append(problems, fmt.Sprintf(
				"%s is listed red by design and is %s on %s, a check the list does not name", entry.ID, result.outcome, result.id))
		}
		if isNamed {
			named[result.id] = true
		}
	}
	for _, check := range entry.Checks {
		if !named[check] {
			problems = append(problems, fmt.Sprintf(
				"%s is listed red by design on %s, and the run holds no such result", entry.ID, check))
		}
	}
	return problems
}
