package check

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/guardana/playground/internal/record"
)

// The record says a list was reported only where the report named it, and
// carries every value as the report wrote it.
func TestTheRecordKeepsWhatTheReportLeftOutApartFromWhatItNamedEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "step-1.json")
	body := `{"schema_version": 18, "run": {"target": {"ref": "target"},
 "result_summary": {"rules_run": ["lab.rule.a"], "gate": null}, "coverage": {"shortfall": null}},
 "scope": null,
 "findings": [{"rule_id": "lab.rule.a", "severity": "LOW", "target_ref": "target/x:3", "evidence": {"summary": "s"}}],
 "unverified": [], "waived": [], "errors": null}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	ran, err := readReport(path, "target")
	if err != nil {
		t.Fatal(err)
	}
	want := record.Run{
		Target:   "target",
		RulesRun: record.List[string]{Items: []string{"lab.rule.a"}, Reported: true},
		Findings: record.List[record.Finding]{Reported: true, Items: []record.Finding{
			{Rule: "lab.rule.a", Severity: "LOW", Target: "target/x:3", Summary: "s"},
		}},
		Unverified: record.List[record.Finding]{Reported: true},
		Waived:     record.List[record.Finding]{Reported: true},
	}
	if !reflect.DeepEqual(ran, want) {
		t.Errorf("record\n got %+v\nwant %+v", ran, want)
	}
}
