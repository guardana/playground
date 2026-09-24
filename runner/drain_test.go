package main

import (
	"context"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
)

// drainedHealth is the enforcer's /healthz with everything handed over and
// nothing lost, in the shape its health handler writes at the pin.
const drainedHealth = `{"status":"ok","spool":{"unacknowledged":0,"quarantined_records":0,"quarantined_bytes":0,"truncated":0},` +
	`"exporter":{"acknowledged":12,"quarantined":0,"partial_rejected":0,"refused":0}}`

// A spool that let go of its records by quarantining them, or lost some to a
// truncation, holds nothing unacknowledged and still did not hand the trail
// over; neither did an exporter that stopped or had records refused.
func TestADrainThatLostRecordsFails(t *testing.T) {
	for name, health := range map[string]string{
		"records quarantined in the spool": strings.Replace(drainedHealth, `"quarantined_records":0`, `"quarantined_records":2`, 1),
		"a truncated spool":                strings.Replace(drainedHealth, `"truncated":0`, `"truncated":512`, 1),
		"records the exporter quarantined": strings.Replace(drainedHealth, `"quarantined":0,`, `"quarantined":1,`, 1),
		"records the collector refused":    strings.Replace(drainedHealth, `"refused":0`, `"refused":1`, 1),
		"records the collector dropped":    strings.Replace(drainedHealth, `"partial_rejected":0`, `"partial_rejected":1`, 1),
		"an exporter that stopped":         strings.Replace(drainedHealth, `"refused":0}`, `"refused":0,"stopped":"spool closed"}`, 1),
		"no quarantine count":              strings.Replace(drainedHealth, `"quarantined_records":0,`, ``, 1),
		"no truncation count":              strings.Replace(drainedHealth, `,"truncated":0`, ``, 1),
		"no exporter":                      strings.Replace(drainedHealth, `,"exporter":{"acknowledged":12,"quarantined":0,"partial_rejected":0,"refused":0}`, ``, 1),
		"no exporter quarantine count":     strings.Replace(drainedHealth, `"quarantined":0,`, ``, 1),
		"no exporter refusal count":        strings.Replace(drainedHealth, `,"refused":0`, ``, 1),
		"a count that is not a number":     strings.Replace(drainedHealth, `"truncated":0`, `"truncated":"0"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if health == drainedHealth {
				t.Fatal("the mutation did not apply")
			}
			subject, compose, scenario := enforcerLab(t)
			answer := compose.exec
			compose.exec = func(service string, args []string) Split {
				if strings.HasSuffix(args[len(args)-1], "/healthz") {
					return Split{Stdout: "status 200\n" + health}
				}
				return answer(service, args)
			}
			graded, err := subject.execute(context.Background(), scenario)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if result := results(graded)["plane/drained"]; result.Outcome != assertion.Fail {
				t.Errorf("plane/drained is %s: %s", result.Outcome, result.Got)
			}
			if len(compose.stopped) != 0 {
				t.Errorf("the trail was read after a lossy drain: stopped %v", compose.stopped)
			}
		})
	}
}
