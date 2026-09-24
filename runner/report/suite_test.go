package report_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/guardana/playground/runner/report"
)

type suiteDocument struct {
	Suites []struct {
		Properties []struct {
			Name  string `xml:"name,attr"`
			Value string `xml:"value,attr"`
		} `xml:"properties>property"`
		Cases []struct {
			Classname string `xml:"classname,attr"`
		} `xml:"testcase"`
	} `xml:"testsuite"`
}

// A known gap passes while the system still does what it documents, so CI has
// to be able to tell its green from the catalogue's by the case alone.
func TestJUnitNamesTheSuiteInEveryClassname(t *testing.T) {
	for suite, gap := range map[string]string{"catalogue": "", "known-gap": "the gateway builds no run flow"} {
		t.Run(suite, func(t *testing.T) {
			graded := mixed()
			graded.Gap = gap
			var out strings.Builder
			if err := report.WriteJUnit(&out, graded, report.Provenance{}); err != nil {
				t.Fatalf("WriteJUnit: %v", err)
			}
			var parsed suiteDocument
			if err := xml.Unmarshal([]byte(out.String()), &parsed); err != nil || len(parsed.Suites) != 1 {
				t.Fatalf("the document is not one suite: %v\n%s", err, out.String())
			}
			for _, one := range parsed.Suites[0].Cases {
				if one.Classname != suite+"."+graded.Scenario {
					t.Errorf("classname %q, want %q", one.Classname, suite+"."+graded.Scenario)
				}
			}
			wantGap := map[string]string{"catalogue": "none", "known-gap": gap}[suite]
			found := false
			for _, property := range parsed.Suites[0].Properties {
				if property.Name == "gap" {
					found = property.Value == wantGap
				}
			}
			if !found {
				t.Errorf("no gap property reading %q:\n%s", wantGap, out.String())
			}
		})
	}
}
