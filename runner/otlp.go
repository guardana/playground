package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/runfile"
)

// errNoEvidence reports a collector export that cannot hold a trail: nothing
// was written to it yet, or it decoded cleanly to zero events. The collector
// answers the enforcer's export request before its file exporter has written
// the delivery, so a source read too early is empty rather than absent; a
// caller that wrote evidence.jsonl for either case would leave a trail nobody
// could tell apart from a run that genuinely produced no events.
var errNoEvidence = errors.New("collector export holds no evidence")

// maxCollectorExportBytes bounds the collector's export read into memory: room
// for maxEvents records at the two kilobytes an exported record takes.
const maxCollectorExportBytes = 128 << 20

// writeEvidenceFromCollector reads the collector file exporter's output at
// otlpPath, decodes it under namespace, and writes the run's evidence.jsonl at
// evidencePath, one event per line, so every check can cite file:line.
//
// A decode error, or a source with no evidence to write, is returned as is
// and evidencePath is left untouched: a trail half written, or emptied, on
// top of whatever the path held before is a trail nobody could tell apart
// from a short one, and this reader never manufactures that ambiguity on the
// caller's behalf.
func writeEvidenceFromCollector(otlpPath, evidencePath, namespace string, limit int) error {
	body, err := runfile.ReadRegular(otlpPath, maxCollectorExportBytes)
	if err != nil {
		return fmt.Errorf("reading the collector's export: %w", err)
	}
	if len(body) == 0 {
		return fmt.Errorf("%s: %w", otlpPath, errNoEvidence)
	}

	events, err := evidence.DecodeOTLP(bytes.NewReader(body), namespace, limit)
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
	return write(path, func(file *os.File) error {
		encoder := json.NewEncoder(file)
		for _, event := range events {
			if err := encoder.Encode(event); err != nil {
				return err
			}
		}
		return file.Sync()
	})
}
