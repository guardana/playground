package main

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"

	"github.com/guardana/playground/internal/labspec"
)

// gatewayIdentity is who a scenario's gateway part says the enforcer decides
// for. An unset principal tenant takes the gateway's own tenant_id.
type gatewayIdentity struct {
	TenantID    string `json:"tenant_id"`
	Environment string `json:"environment"`
	Listener    struct {
		Principal struct {
			ID       string `json:"id"`
			TenantID string `json:"tenant_id"`
		} `json:"principal"`
		Agent struct {
			ID string `json:"id"`
		} `json:"agent"`
	} `json:"listener"`
}

// refuseAnotherIdentity holds the trajectory's principal, agent and
// environment to the gateway part's. The trajectory's are sent nowhere: the
// listener authenticates nobody and decides every call for the identity its
// configuration names, so a trajectory naming another would describe a run
// the enforcer never decided.
func refuseAnotherIdentity(space workspace, spec labspec.Scenario, trajectory labspec.Trajectory) error {
	if spec.Gateway == nil {
		return nil
	}
	body, err := os.ReadFile(space.file(spec.Gateway.Config)) // #nosec G304 -- a workspace path the scenario names.
	if err != nil {
		return err
	}
	var gateway gatewayIdentity
	if err := yaml.Unmarshal(body, &gateway); err != nil {
		return fmt.Errorf("gateway.config %s: %w", spec.Gateway.Config, err)
	}
	tenant := gateway.Listener.Principal.TenantID
	if tenant == "" {
		tenant = gateway.TenantID
	}
	for _, field := range []struct{ name, trajectory, gateway string }{
		{"principal.id", trajectory.Principal.ID, gateway.Listener.Principal.ID},
		{"principal.tenant_id", trajectory.Principal.TenantID, tenant},
		{"agent.id", trajectory.Agent.ID, gateway.Listener.Agent.ID},
		{"session.environment", trajectory.Session.Environment, gateway.Environment},
	} {
		if field.gateway != "" && field.trajectory != field.gateway {
			return fmt.Errorf("the trajectory's %s is %q and %s names %q, the one the enforcer decides for",
				field.name, field.trajectory, spec.Gateway.Config, field.gateway)
		}
	}
	return nil
}
