package driver

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/guardana/playground/runner/report"
)

var pinLine = regexp.MustCompile(`^([A-Z][A-Z0-9_]*)=(.*)$`)

// Pins are the variables of versions.env, in file order.
type Pins []report.Pin

// ReadPins reads versions.env. A line that is not a comment, not blank and not
// NAME=VALUE is refused rather than skipped.
func ReadPins(path string) (Pins, error) {
	file, err := os.Open(path) // #nosec G304 -- the lab's own versions.env.
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var pins Pins
	scanner := bufio.NewScanner(file)
	for number := 1; scanner.Scan(); number++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		match := pinLine.FindStringSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("%s:%d: not NAME=VALUE: %q", path, number, line)
		}
		pins = append(pins, report.Pin{Name: match[1], Value: match[2]})
	}
	return pins, scanner.Err()
}

// Value is the pin named name, empty when versions.env names none.
func (p Pins) Value(name string) string {
	for _, pin := range p {
		if pin.Name == name {
			return pin.Value
		}
	}
	return ""
}
