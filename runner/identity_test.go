package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

const identityGateway = `mode: ENFORCE
project_id: lab
tenant_id: tenant_a
environment: development
listener:
  principal:
    id: user_123
    tenant_id: tenant_a
  agent:
    id: support-agent
`

func identityTrajectory() labspec.Trajectory {
	return labspec.Trajectory{
		Agent:     labspec.Agent{ID: "support-agent", Framework: "scripted"},
		Principal: labspec.Principal{ID: "user_123", Type: "human", TenantID: "tenant_a"},
		Session:   labspec.Session{Environment: "development"},
	}
}

func identityRefusal(t *testing.T, gateway string, trajectory labspec.Trajectory) error {
	t.Helper()
	root := t.TempDir()
	writeFile(filepath.Join(root, "config/gateway/scenarios/case.yaml"), gateway)
	spec := labspec.Scenario{Gateway: &labspec.Gateway{Config: "config/gateway/scenarios/case.yaml"}}
	return refuseAnotherIdentity(workspace{dir: root}, spec, trajectory)
}

// The listener authenticates nobody: the enforcer decides every call for the
// principal and agent its configuration names, whatever the trajectory says.
func TestATrajectoryNamesTheIdentityTheEnforcerDecidesFor(t *testing.T) {
	if err := identityRefusal(t, identityGateway, identityTrajectory()); err != nil {
		t.Fatalf("a trajectory naming the listener's identity was refused: %v", err)
	}
	for name, change := range map[string]func(*labspec.Trajectory){
		"another principal":   func(tr *labspec.Trajectory) { tr.Principal.ID = "user_412" },
		"another tenant":      func(tr *labspec.Trajectory) { tr.Principal.TenantID = "tenant_b" },
		"another agent":       func(tr *labspec.Trajectory) { tr.Agent.ID = "other-agent" },
		"another environment": func(tr *labspec.Trajectory) { tr.Session.Environment = "production" },
	} {
		t.Run(name, func(t *testing.T) {
			trajectory := identityTrajectory()
			change(&trajectory)
			err := identityRefusal(t, identityGateway, trajectory)
			if err == nil || !strings.Contains(err.Error(), "the enforcer decides") {
				t.Errorf("a trajectory naming %s was accepted: %v", name, err)
			}
		})
	}
}

// With no tenant of its own the listener's principal takes the gateway's
// tenant_id, so that is the tenant the trajectory is held to.
func TestAnUnsetPrincipalTenantIsTheGatewaysOwn(t *testing.T) {
	gateway := strings.Replace(identityGateway, "    tenant_id: tenant_a\n", "", 1)
	trajectory := identityTrajectory()
	if err := identityRefusal(t, gateway, trajectory); err != nil {
		t.Errorf("the gateway's own tenant was refused: %v", err)
	}
	trajectory.Principal.TenantID = "tenant_b"
	if err := identityRefusal(t, gateway, trajectory); err == nil {
		t.Error("a tenant other than the gateway's own was accepted")
	}
}
