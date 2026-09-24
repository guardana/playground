package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// refuseOverriddenPins fails when the environment sets a variable versions.env
// pins. Compose lets the environment win over its env file, so the run would use
// that value while the report header prints the file's.
func refuseOverriddenPins(root string, environ []string) error {
	pins, err := readPins(filepath.Join(root, versionFile))
	if err != nil {
		return fmt.Errorf("the pins cannot be read: %w", err)
	}
	set := map[string]bool{}
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		set[name] = true
	}
	var overridden []string
	for _, pin := range pins {
		if set[pin.Name] {
			overridden = append(overridden, pin.Name)
		}
	}
	if len(overridden) > 0 {
		return fmt.Errorf("the environment sets %s, which %s pins; unset it so the run uses the pin its report names",
			strings.Join(overridden, ", "), versionFile)
	}
	return nil
}
