package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/guardana/playground/internal/evidence"
)

// errNoEvidence reports a collector export that cannot hold a trail: nothing
// was written to it yet, or it decoded cleanly to zero events. The collector
// answers the enforcer's export request before its file exporter has written
// the delivery, so a source read too early is empty rather than absent; a
// caller that wrote evidence.jsonl for either case would leave a trail nobody
// could tell apart from a run that genuinely produced no events.
var errNoEvidence = errors.New("collector export holds no evidence")

// writeEvidenceFromCollector reads the collector file exporter's output at
// otlpPath, decodes it under namespace, and writes the run's evidence.jsonl at
// evidencePath, one event per line, so every check keeps citing file:line the
// same way it does against the stub gateway's own JSONL.
//
// A decode error, or a source with no evidence to write, is returned as is
// and evidencePath is left untouched: a trail half written, or emptied, on
// top of whatever the path held before is a trail nobody could tell apart
// from a short one, and this reader never manufactures that ambiguity on the
// caller's behalf.
func writeEvidenceFromCollector(otlpPath, evidencePath, namespace string, limit int) error {
	file, err := os.Open(otlpPath) // #nosec G304,G703 -- the path is inside the run directory the runner made.
	if err != nil {
		return fmt.Errorf("opening the collector's export: %w", err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("statting the collector's export: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("%s: %w", otlpPath, errNoEvidence)
	}

	events, err := evidence.DecodeOTLP(file, namespace, limit)
	if err != nil {
		return fmt.Errorf("decoding the collector's export: %w", err)
	}
	if len(events) == 0 {
		return fmt.Errorf("%s: %w", otlpPath, errNoEvidence)
	}
	if err := writeEventsJSONL(evidencePath, events); err != nil {
		return fmt.Errorf("writing %s: %w", evidencePath, err)
	}
	return nil
}

// writeEventsJSONL writes through a temporary file, fsyncs it and renames it
// into place, so a reader of evidencePath never observes a partial write:
// either the file still holds what it held before this call, or it holds
// exactly this trail, durably. The temporary file is removed whether the
// write or the rename is what failed, so a repeated run never trips over a
// stale one.
func writeEventsJSONL(path string, events []evidence.Event) error {
	tmp := path + ".tmp"
	if err := writeEventsFile(tmp, events); err != nil {
		_ = os.Remove(tmp) // #nosec G703 -- inside the run directory the runner made.
		return err
	}
	if err := os.Rename(tmp, path); err != nil { // #nosec G703 -- inside the run directory the runner made.
		_ = os.Remove(tmp) // #nosec G703 -- inside the run directory the runner made.
		return err
	}
	return nil
}

func writeEventsFile(path string, events []evidence.Event) error {
	// #nosec G304,G703 -- the path is inside the run directory the runner made.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return errors.Join(err, file.Close())
		}
	}
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	return file.Close()
}
