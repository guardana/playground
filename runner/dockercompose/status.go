package dockercompose

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/guardana/playground/internal/assertion"
)

// container is the part of `docker compose ps --format json` the runner reads.
type container struct {
	Service  string `json:"Service"`
	State    string `json:"State"`
	Health   string `json:"Health"`
	ExitCode int    `json:"ExitCode"`
}

// parseStatus turns compose's own output into one record per service the runner
// asked about. A service compose said nothing about is recorded as not running:
// a silence is what a check must never mistake for success.
func parseStatus(output string, wanted []string) ([]assertion.Service, error) {
	reported, err := decodeContainers(output)
	if err != nil {
		return nil, err
	}
	services := make([]assertion.Service, 0, len(wanted))
	for _, name := range wanted {
		found, ok := reported[name]
		if !ok {
			services = append(services, assertion.Service{
				Name:   name,
				Detail: "no container: compose reported nothing for this service",
			})
			continue
		}
		services = append(services, assertion.Service{
			Name:    name,
			Running: found.State == "running" && (found.Health == "" || found.Health == "healthy"),
			Detail:  describeContainer(found),
		})
	}
	return services, nil
}

func describeContainer(found container) string {
	detail := found.State
	if found.State != "running" {
		detail = fmt.Sprintf("%s (%d)", found.State, found.ExitCode)
	}
	if found.Health != "" {
		detail += ", health " + found.Health
	}
	return detail
}

// decodeContainers reads either shape compose writes: one object per line, or
// one array. Which one it is depends on the compose version, and a runner that
// read only one of them would report a whole profile as absent.
func decodeContainers(output string) (map[string]container, error) {
	reported := make(map[string]container)
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return reported, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var all []container
		if err := json.Unmarshal([]byte(trimmed), &all); err != nil {
			return nil, fmt.Errorf("reading compose ps: %w", err)
		}
		for _, found := range all {
			reported[found.Service] = found
		}
		return reported, nil
	}
	for _, line := range strings.Split(trimmed, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var found container
		if err := json.Unmarshal([]byte(line), &found); err != nil {
			return nil, fmt.Errorf("reading compose ps: %w", err)
		}
		reported[found.Service] = found
	}
	return reported, nil
}
