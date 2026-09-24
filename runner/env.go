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
	}
	if absolute, err := filepath.Abs(l.reports); err == nil {
		env["LAB_REPORTS_HOST_DIR"] = absolute
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
