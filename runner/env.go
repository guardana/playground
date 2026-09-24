package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/guardana/playground/internal/labspec"
)

// environment is what compose interpolates into the topology for this run.
func (l lab) environment(spec labspec.Scenario, runID, runDir string) map[string]string {
	env := map[string]string{
		"LAB_RUN_ID":           runID,
		"LAB_REPORTS_DIR":      containerReports,
		"COMPOSE_PROJECT_NAME": projectName(runID),
		workspaceVariable:      l.workspace.dir,
	}
	if absolute, err := filepath.Abs(runDir); err == nil {
		env["LAB_RUN_HOST_DIR"] = absolute
	}
	if spec.Stub.Verdicts != "" {
		env["LAB_STUB_VERDICTS"] = inContainer(spec.Stub.Verdicts)
	}
	return env
}

// projectName gives every run its own compose project, so two runs share no
// container, network or volume and a run that crashed leaves nothing the next
// one boots into. The random suffix of the run id is what makes it unique.
func projectName(runID string) string {
	return "lab-" + strings.ToLower(runID[strings.LastIndex(runID, "-")+1:])
}

// enforcerNamespace is the namespace versions.env pins for the enforcer. Its
// gateway marks the answers it makes itself under it, and the agent is told it
// so an upstream result shaped like a pending answer is not retried as one.
func enforcerNamespace(root string) (string, error) {
	pins, err := readPins(filepath.Join(root, versionFile))
	if err != nil {
		return "", fmt.Errorf("the pins cannot be read: %w", err)
	}
	namespace := pinValue(pins, "ENFORCER_NAMESPACE")
	if namespace == "" {
		return "", fmt.Errorf("%s pins no ENFORCER_NAMESPACE", versionFile)
	}
	return namespace, nil
}

// enforcerPin is the commit versions.env pins the enforcer at.
func enforcerPin(root string) (string, error) {
	pins, err := readPins(filepath.Join(root, versionFile))
	if err != nil {
		return "", fmt.Errorf("the pins cannot be read: %w", err)
	}
	pin := pinValue(pins, "ENFORCER_COMMIT")
	if pin == "" {
		return "", fmt.Errorf("%s pins no ENFORCER_COMMIT", versionFile)
	}
	return pin, nil
}

// enforcerImage is the enforcer's image as versions.env pins it, name and tag.
func enforcerImage(root string) (string, error) {
	pins, err := readPins(filepath.Join(root, versionFile))
	if err != nil {
		return "", fmt.Errorf("the pins cannot be read: %w", err)
	}
	name, commit := pinValue(pins, "ENFORCER_IMAGE"), pinValue(pins, "ENFORCER_COMMIT")
	if name == "" || commit == "" {
		return "", fmt.Errorf("%s pins no ENFORCER_IMAGE or no ENFORCER_COMMIT", versionFile)
	}
	return name + ":" + commit, nil
}

// lookupIn reads one variable from an environment given as NAME=VALUE lines.
func lookupIn(environ []string) func(string) string {
	return func(name string) string {
		for _, entry := range environ {
			if key, value, ok := strings.Cut(entry, "="); ok && key == name {
				return value
			}
		}
		return ""
	}
}
