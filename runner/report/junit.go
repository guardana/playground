// Package report writes what a run established, once for a machine and once
// for a person.
//
// The JUnit document has no <skipped> element in it, ever. Every tool that
// reads JUnit treats a skipped case as "not a failure", and an indeterminate
// result is a run that established nothing — handing that to CI as a skip is
// how a lab that proved nothing gets read as green. Indeterminate is written as
// a failure whose message says so.
package report

import (
	"encoding/xml"
	"fmt"
	"io"

	"github.com/guardana/playground/internal/assertion"
)

// WriteJUnit writes the report as one JUnit suite, one testcase per result.
func WriteJUnit(w io.Writer, r assertion.Report, p Provenance) error {
	results := r.Results
	if len(results) == 0 {
		// An empty suite is green to every reader of JUnit, and a report with
		// no results is the opposite of green.
		results = []assertion.Result{{
			Check:   "report/results",
			Outcome: assertion.Indeterminate,
			Want:    "at least one check to have read a record",
			Got:     "no result",
			Detail:  "the run produced no result, so it graded nothing",
		}}
	}

	suite := junitSuite{
		Name:      r.Scenario,
		Timestamp: r.StartedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Time:      seconds(r),
		Properties: append([]junitProperty{
			{Name: "run-id", Value: r.RunID},
			{Name: "outcome", Value: r.Outcome().String()},
			{Name: "gap", Value: or(r.Gap, "none")},
		}, provenanceProperties(p)...),
	}
	for _, result := range results {
		suite.Cases = append(suite.Cases, testCase(r.Suite()+"."+r.Scenario, result))
		suite.Tests++
		if result.Outcome != assertion.Pass {
			suite.Failures++
		}
	}

	document := junitSuites{
		Name:     r.Scenario,
		Tests:    suite.Tests,
		Failures: suite.Failures,
		Time:     suite.Time,
		Suites:   []junitSuite{suite},
	}
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// testCase writes one result; classname carries the suite before the scenario,
// so a reader of the JUnit alone can tell a known gap's pass from the catalogue's.
func testCase(classname string, result assertion.Result) junitCase {
	one := junitCase{Name: result.Check, Classname: classname, Time: "0"}
	if result.Outcome == assertion.Pass {
		return one
	}
	message := fmt.Sprintf("want %s, got %s", or(result.Want, "an expectation nobody wrote"), or(result.Got, "nothing"))
	if result.Outcome == assertion.Indeterminate {
		message = "nothing was established: " + message
	}
	one.Failure = &junitFailure{
		Message: message,
		Type:    result.Outcome.String(),
		Body:    fmt.Sprintf("%s\nread from: %s", or(result.Detail, message), or(result.Source, "nothing was named")),
	}
	return one
}

func seconds(r assertion.Report) string {
	elapsed := r.EndedAt.Sub(r.StartedAt).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	return fmt.Sprintf("%.3f", elapsed)
}

func or(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

type junitSuites struct {
	XMLName  xml.Name     `xml:"testsuites"`
	Name     string       `xml:"name,attr"`
	Tests    int          `xml:"tests,attr"`
	Failures int          `xml:"failures,attr"`
	Errors   int          `xml:"errors,attr"`
	Time     string       `xml:"time,attr"`
	Suites   []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	XMLName    xml.Name        `xml:"testsuite"`
	Name       string          `xml:"name,attr"`
	Tests      int             `xml:"tests,attr"`
	Failures   int             `xml:"failures,attr"`
	Errors     int             `xml:"errors,attr"`
	Time       string          `xml:"time,attr"`
	Timestamp  string          `xml:"timestamp,attr"`
	Properties []junitProperty `xml:"properties>property,omitempty"`
	Cases      []junitCase     `xml:"testcase"`
}

type junitProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type junitCase struct {
	XMLName   xml.Name      `xml:"testcase"`
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
}

type junitFailure struct {
	XMLName xml.Name `xml:"failure"`
	Message string   `xml:"message,attr"`
	Type    string   `xml:"type,attr"`
	Body    string   `xml:",chardata"`
}
