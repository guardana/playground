package labspec_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

const policyLine = "  policy: config/policies/case-01-allow-read.json\n"

func TestAScenarioPutsAnUpstreamInATenantOfItsOwn(t *testing.T) {
	body := strings.Replace(heldScenario, policyLine, policyLine+"  upstream_tenants: { victim-crm: tenant_b }\n", 1)
	scenario, err := labspec.LoadScenario(writeFile(t, "case-01-allow-read.yaml", body))
	if err != nil || scenario.Gateway.UpstreamTenants["victim-crm"] != "tenant_b" {
		t.Fatalf("upstream tenants did not load: %+v, %v", scenario.Gateway, err)
	}
	for name, tenants := range map[string]string{
		"not a victim":          "{ collector: tenant_b }",
		"a victim and a path":   "{ victim-crm/crm.read_customer: tenant_b }",
		"an empty tenant":       "{ victim-crm: '' }",
		"a tenant with a space": "{ victim-crm: 'tenant b' }",
		"a tenant with a colon": "{ victim-crm: 'tenant:b' }",
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.Replace(heldScenario, policyLine, policyLine+"  upstream_tenants: "+tenants+"\n", 1)
			if _, err := labspec.LoadScenario(writeFile(t, "case-01-allow-read.yaml", body)); !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}
