package report

import (
	"fmt"

	"github.com/guardana/playground/internal/record"
)

// Pin is the approved manifest a write_pin step leaves.
type Pin struct {
	SchemaVersion int               `json:"schema_version"`
	Server        string            `json:"server"`
	Tools         map[string]string `json:"tools"`
}

// ReadPin reads a pin of the schema this lab reads. Whether it approves the
// probed server, and any tool at all, is the grader's to judge.
func ReadPin(path string) (Pin, error) {
	var pin Pin
	if err := readFile(path, &pin); err != nil {
		return Pin{}, fmt.Errorf("the pin cannot be read: %w", err)
	}
	if pin.SchemaVersion != pinSchema {
		return Pin{}, fmt.Errorf("the pin is schema %d and the lab reads %d", pin.SchemaVersion, pinSchema)
	}
	return pin, nil
}

// Approval is the pin as a neutral record.
func (p Pin) Approval() record.Approval {
	return record.Approval{Server: p.Server, Tools: len(p.Tools)}
}
