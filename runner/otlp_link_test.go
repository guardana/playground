package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The collector writes its export into a directory every service can write,
// so a link there is refused rather than decoded as the run's trail.
func TestACollectorExportThatIsALinkIsRefused(t *testing.T) {
	dir := t.TempDir()
	target, err := filepath.Abs(otlpFixture)
	if err != nil {
		t.Fatal(err)
	}
	otlpPath := filepath.Join(dir, "otlp-logs.json")
	if err := os.Symlink(target, otlpPath); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(dir, "evidence.jsonl")
	if err := writeEvidenceFromCollector(otlpPath, evidencePath, otlpGoldenNS, otlpGoldenSize); err == nil {
		t.Fatal("a collector export that is a link was decoded into the trail")
	}
	if _, err := os.Lstat(evidencePath); !os.IsNotExist(err) {
		t.Errorf("evidence.jsonl was written from a link: %v", err)
	}
}
