package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/guardana/playground/internal/evidence"
)

const (
	otlpFixture    = "../internal/evidence/testdata/otlp/export_logs_request.json"
	otlpGoldenNS   = "guardana.control"
	otlpGoldenSize = 4
)

// The golden is the enforcer's own exporter test fixture: what its collector
// file export holds. Decoding it once with evidence.DecodeOTLP and once
// through this writer and a re-read of what it wrote is the same proof the
// package-level golden test makes, one layer further out.
func TestWriteEvidenceFromCollectorMatchesTheGolden(t *testing.T) {
	dir := t.TempDir()
	evidencePath := filepath.Join(dir, "evidence.jsonl")

	if err := writeEvidenceFromCollector(otlpFixture, evidencePath, otlpGoldenNS, otlpGoldenSize); err != nil {
		t.Fatalf("writeEvidenceFromCollector: %v", err)
	}

	want := decodeFixture(t)
	got := readEvidenceFile(t, evidencePath)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("evidence.jsonl holds %+v,\nwant %+v", got, want)
	}
}

// A decode error must never leave a caller unable to tell "nothing wrote a
// trail" from "something wrote half of one" apart: evidencePath keeps
// whatever it held before the call.
func TestWriteEvidenceFromCollectorLeavesTheDestinationUntouchedOnError(t *testing.T) {
	dir := t.TempDir()
	evidencePath := filepath.Join(dir, "evidence.jsonl")
	const before = `{"eventId":"kept","kind":"EVENT_KIND_ACTION_PROPOSED","requestId":"r"}` + "\n"
	if err := os.WriteFile(evidencePath, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}

	malformed := filepath.Join(dir, "malformed.json")
	if err := os.WriteFile(malformed, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := writeEvidenceFromCollector(malformed, evidencePath, otlpGoldenNS, otlpGoldenSize)
	if err == nil {
		t.Fatal("writeEvidenceFromCollector: got no error decoding malformed input")
	}

	after, readErr := os.ReadFile(evidencePath) // #nosec G304 -- the path is a t.TempDir() fixture.
	if readErr != nil {
		t.Fatalf("reading evidencePath back: %v", readErr)
	}
	if string(after) != before {
		t.Errorf("evidencePath holds %q after a decode error, want it untouched at %q", after, before)
	}
	if _, err := os.Stat(evidencePath + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("a temporary file was left behind: %v", err)
	}
}

// A decode error is the common case, and it never reaches the write step at
// all. The write step has its own failure mode, tested separately: a
// directory that refuses the temporary file still leaves evidencePath as it
// was, because the write goes through a rename rather than truncating it in
// place.
func TestWriteEvidenceFromCollectorLeavesTheDestinationUntouchedOnAWriteFailure(t *testing.T) {
	dir := t.TempDir()
	evidencePath := filepath.Join(dir, "evidence.jsonl")
	const before = `{"eventId":"kept","kind":"EVENT_KIND_ACTION_PROPOSED","requestId":"r"}` + "\n"
	if err := os.WriteFile(evidencePath, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil { // #nosec G302 -- denying write is the point of this test.
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) // #nosec G302 -- t.TempDir() cleanup needs it back.

	err := writeEvidenceFromCollector(otlpFixture, evidencePath, otlpGoldenNS, otlpGoldenSize)
	if err == nil {
		t.Fatal("writeEvidenceFromCollector: got no error writing a temporary file into a read-only directory")
	}
	after, readErr := os.ReadFile(evidencePath) // #nosec G304 -- the path is a t.TempDir() fixture.
	if readErr != nil {
		t.Fatalf("reading evidencePath back: %v", readErr)
	}
	if string(after) != before {
		t.Errorf("evidencePath holds %q after a write failure, want it untouched at %q", after, before)
	}
}

// A file that never exists cannot be missed because the OTLP source path
// itself was never opened; the namespace it would have been decoded under
// never comes into it.
func TestWriteEvidenceFromCollectorRefusesAnAbsentSource(t *testing.T) {
	dir := t.TempDir()
	err := writeEvidenceFromCollector(filepath.Join(dir, "absent.json"), filepath.Join(dir, "evidence.jsonl"), "unused.namespace", 1)
	if err == nil {
		t.Fatal("writeEvidenceFromCollector: got no error for an absent source file")
	}
}

// The collector answers the enforcer's export request before its file
// exporter has written the delivery, so a source read at that moment is a
// zero-byte file, not an absent one. Writing evidence.jsonl for it would be
// indistinguishable from a genuinely short trail.
func TestWriteEvidenceFromCollectorRefusesAZeroByteSource(t *testing.T) {
	dir := t.TempDir()
	otlpPath := filepath.Join(dir, "otlp-logs.json")
	if err := os.WriteFile(otlpPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(dir, "evidence.jsonl")

	err := writeEvidenceFromCollector(otlpPath, evidencePath, otlpGoldenNS, otlpGoldenSize)
	if !errors.Is(err, errNoEvidence) {
		t.Fatalf("writeEvidenceFromCollector on a zero-byte source: err = %v, want errNoEvidence", err)
	}
	if _, statErr := os.Stat(evidencePath); !os.IsNotExist(statErr) {
		t.Errorf("evidence.jsonl was written from a zero-byte source: %v", statErr)
	}
}

// A well-formed export with no log records decodes cleanly to zero events;
// that is as unusable as a zero-byte source and gets the same refusal rather
// than an empty evidence.jsonl a check could mistake for a trail nothing
// happened on.
func TestWriteEvidenceFromCollectorRefusesAnExportWithNoEvents(t *testing.T) {
	dir := t.TempDir()
	otlpPath := filepath.Join(dir, "otlp-logs.json")
	if err := os.WriteFile(otlpPath, []byte(`{"resourceLogs":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(dir, "evidence.jsonl")

	err := writeEvidenceFromCollector(otlpPath, evidencePath, otlpGoldenNS, otlpGoldenSize)
	if !errors.Is(err, errNoEvidence) {
		t.Fatalf("writeEvidenceFromCollector on an eventless export: err = %v, want errNoEvidence", err)
	}
	if _, statErr := os.Stat(evidencePath); !os.IsNotExist(statErr) {
		t.Errorf("evidence.jsonl was written from an eventless export: %v", statErr)
	}
}

// A rename that fails (the destination exists as a directory) must not leave
// the temporary file behind for the next run to trip over.
func TestWriteEventsJSONLRemovesTheTemporaryFileWhenRenameFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evidence.jsonl")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	err := writeEventsJSONL(path, nil)
	if err == nil {
		t.Fatal("writeEventsJSONL: got no error renaming onto an existing directory")
	}
	if _, statErr := os.Stat(path + ".tmp"); !os.IsNotExist(statErr) {
		t.Errorf("a temporary file was left behind after a rename failure: %v", statErr)
	}
}

func decodeFixture(t *testing.T) []evidence.Event {
	t.Helper()
	file, err := os.Open(otlpFixture) // #nosec G304 -- a fixture under the module.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	events, err := evidence.DecodeOTLP(file, otlpGoldenNS, otlpGoldenSize)
	if err != nil {
		t.Fatalf("evidence.DecodeOTLP: %v", err)
	}
	return events
}

func readEvidenceFile(t *testing.T, path string) []evidence.Event {
	t.Helper()
	file, err := os.Open(path) // #nosec G304 -- a t.TempDir() fixture.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	events, err := evidence.DecodeJSONL(file, otlpGoldenSize)
	if err != nil {
		t.Fatalf("evidence.DecodeJSONL: %v", err)
	}
	return events
}
