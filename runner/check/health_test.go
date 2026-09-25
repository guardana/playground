package check_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

// fullSpoolHealth is the pipeline part of the enforcer's /healthz after two
// starts the full spool refused, in the shape its health handler writes at
// the pin.
const fullSpoolHealth = `{"status":"ok","pipeline":{"blocked":2,"blocks":{"EVIDENCE_UNAVAILABLE":2},` +
	`"executed":0,"reads_unrecorded":0,"sink_failures_after_effect":0,"sink_failures_before_effect":2}}`

func count(n int) *int { return &n }

func gradeHealth(t *testing.T, expect labspec.HealthExpectation, body *string) map[string]assertion.Result {
	t.Helper()
	source := filepath.Join(t.TempDir(), "healthz.json")
	if body != nil {
		if err := os.WriteFile(source, []byte(*body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	results, err := check.Health{Expect: expect, Source: source}.Run(context.Background(), assertion.Records{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	graded := map[string]assertion.Result{}
	for _, result := range results {
		if result.Source != source {
			t.Errorf("%s cites %q, want the record %q", result.Check, result.Source, source)
		}
		graded[result.Check] = result
	}
	return graded
}

var everyHealthField = labspec.HealthExpectation{
	Blocks:                   map[string]int{"EVIDENCE_UNAVAILABLE": 2},
	ReadsUnrecorded:          count(0),
	SinkFailuresBeforeEffect: count(2),
}

var everyHealthCheck = []string{
	"health/blocks/EVIDENCE_UNAVAILABLE", "health/reads_unrecorded", "health/sink_failures_before_effect",
}

func TestHealthPassesOnTheExactCounts(t *testing.T) {
	body := fullSpoolHealth
	graded := gradeHealth(t, everyHealthField, &body)
	if len(graded) != len(everyHealthCheck) {
		t.Fatalf("graded %d, want %v", len(graded), everyHealthCheck)
	}
	for _, name := range everyHealthCheck {
		if result := graded[name]; result.Outcome != assertion.Pass {
			t.Errorf("%s is %s: %s %s", name, result.Outcome, result.Got, result.Detail)
		}
	}
}

// A count one away from the stated one is a different run.
func TestHealthFailsOnACountThatIsNotExact(t *testing.T) {
	for name, mutate := range map[string]func(*labspec.HealthExpectation){
		"blocks":                      func(e *labspec.HealthExpectation) { e.Blocks = map[string]int{"EVIDENCE_UNAVAILABLE": 1} },
		"reads_unrecorded":            func(e *labspec.HealthExpectation) { e.ReadsUnrecorded = count(1) },
		"sink_failures_before_effect": func(e *labspec.HealthExpectation) { e.SinkFailuresBeforeEffect = count(3) },
	} {
		t.Run(name, func(t *testing.T) {
			expect := everyHealthField
			mutate(&expect)
			body := fullSpoolHealth
			failed := 0
			for check, result := range gradeHealth(t, expect, &body) {
				if result.Outcome == assertion.Fail {
					failed++
					if !strings.Contains(check, name) {
						t.Errorf("%s failed, want only the %s check to", check, name)
					}
				}
			}
			if failed != 1 {
				t.Errorf("%d checks failed, want one", failed)
			}
		})
	}
}

// A reason the answer never counted reads as zero blocks of it.
func TestHealthReadsAReasonNeverCountedAsZero(t *testing.T) {
	body := fullSpoolHealth
	graded := gradeHealth(t, labspec.HealthExpectation{Blocks: map[string]int{"RULE_DENY": 1}}, &body)
	if result := graded["health/blocks/RULE_DENY"]; result.Outcome != assertion.Fail || result.Got != "0" {
		t.Errorf("health/blocks/RULE_DENY is %s, got %q", result.Outcome, result.Got)
	}
}

// Without a record, or with one that does not say, nothing was established.
func TestHealthFailsWithoutARecordThatSays(t *testing.T) {
	unparsed := `{"pipeline":`
	noPipeline := `{"status":"ok"}`
	noCounters := `{"pipeline":{"blocked":2}}`
	notNumbers := strings.ReplaceAll(fullSpoolHealth, `":2`, `":"2"`)
	oversized := fullSpoolHealth + strings.Repeat(" ", 1<<20)
	for name, body := range map[string]*string{
		"no record": nil, "unparsed": &unparsed, "no pipeline": &noPipeline,
		"no counters": &noCounters, "counts that are not numbers": &notNumbers,
		"a record past its bound": &oversized,
	} {
		t.Run(name, func(t *testing.T) {
			graded := gradeHealth(t, everyHealthField, body)
			if len(graded) != len(everyHealthCheck) {
				t.Fatalf("graded %v, want %v", graded, everyHealthCheck)
			}
			for _, name := range everyHealthCheck {
				result := graded[name]
				if result.Outcome != assertion.Fail {
					t.Errorf("%s is %s: %s", name, result.Outcome, result.Got)
				}
				if body != &noCounters && result.Detail == "" {
					t.Errorf("%s fails without saying why the record was not read", name)
				}
			}
		})
	}
}
