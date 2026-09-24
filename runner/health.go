package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var errLossyDrain = errors.New("the plane did not hand its whole trail over")

// planeHealth is the part of the enforcer's /healthz the drain reads, in the
// shape its health handler writes at the pin. Every count is a pointer: a
// field the answer lacks is a shape the lab does not know, never a zero.
type planeHealth struct {
	Spool *struct {
		Unacknowledged     *int64 `json:"unacknowledged"`
		QuarantinedRecords *int64 `json:"quarantined_records"`
		Truncated          *int64 `json:"truncated"`
	} `json:"spool"`
	Exporter *struct {
		Acknowledged    *int64  `json:"acknowledged"`
		Quarantined     *int64  `json:"quarantined"`
		PartialRejected *int64  `json:"partial_rejected"`
		Refused         *int64  `json:"refused"`
		Stopped         *string `json:"stopped"`
	} `json:"exporter"`
}

type count struct {
	name  string
	value *int64
}

// readHealth parses one /healthz answer and refuses one lacking a field the
// drain reads.
func readHealth(body []byte) (planeHealth, error) {
	var health planeHealth
	if err := json.Unmarshal(body, &health); err != nil {
		return planeHealth{}, fmt.Errorf("%w: its /healthz does not parse: %w", errLossyDrain, err)
	}
	if health.Spool == nil || health.Exporter == nil {
		return planeHealth{}, fmt.Errorf("%w: its /healthz has no spool or no exporter", errLossyDrain)
	}
	var missing []string
	for _, field := range append(health.losses(), count{"spool.unacknowledged", health.Spool.Unacknowledged}) {
		if field.value == nil {
			missing = append(missing, field.name)
		}
	}
	if len(missing) > 0 {
		return planeHealth{}, fmt.Errorf("%w: its /healthz has no %s", errLossyDrain, strings.Join(missing, ", "))
	}
	return health, nil
}

// losses are the counts of records that left the spool without reaching the
// collector's file: quarantined, cut off by a truncation, or refused or
// dropped by the collector.
func (h planeHealth) losses() []count {
	return []count{
		{"spool.quarantined_records", h.Spool.QuarantinedRecords}, {"spool.truncated", h.Spool.Truncated},
		{"exporter.quarantined", h.Exporter.Quarantined}, {"exporter.partial_rejected", h.Exporter.PartialRejected},
		{"exporter.refused", h.Exporter.Refused},
	}
}

// acknowledged is how many records the collector has acknowledged, and
// whether the answer said.
func (h planeHealth) acknowledged() (int64, bool) {
	if h.Exporter == nil || h.Exporter.Acknowledged == nil {
		return 0, false
	}
	return *h.Exporter.Acknowledged, true
}

// unacknowledged is what the spool still holds for the collector.
func (h planeHealth) unacknowledged() int64 { return *h.Spool.Unacknowledged }

// lost names every loss, and an exporter that stopped.
func (h planeHealth) lost() error {
	var lost []string
	for _, field := range h.losses() {
		if *field.value != 0 {
			lost = append(lost, fmt.Sprintf("%s %d", field.name, *field.value))
		}
	}
	if h.Exporter.Stopped != nil {
		lost = append(lost, "the exporter stopped: "+*h.Exporter.Stopped)
	}
	if len(lost) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", errLossyDrain, strings.Join(lost, ", "))
}
