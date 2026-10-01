package labspec_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

// Nothing in the verifier suite boots an enforcer, so a gateway and the
// /healthz counts it would answer would load and be graded by nothing.
func TestAVerifierScenarioRefusesAGateway(t *testing.T) {
	gateway := "profile: [verifier]\ngateway:\n  config: config/gateway/x.yaml\n  policy: config/policies/x.json\n"
	health := "  health: { reads_unrecorded: 0 }\n  verifier:\n    1:"
	for name, replace := range map[string][]string{
		"a gateway":                   {"profile: [verifier]\n", gateway},
		"a gateway and expect.health": {"profile: [verifier]\n", gateway, "  verifier:\n    1:", health},
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.NewReplacer(replace...).Replace(verifierScenario)
			if !strings.Contains(body, "gateway:") {
				t.Fatal("the replacement added no gateway")
			}
			_, err := loadVerifier(t, body)
			if !errors.Is(err, labspec.ErrInvalid) || !strings.Contains(err.Error(), "gateway") {
				t.Fatalf("err = %v, want ErrInvalid naming the gateway", err)
			}
		})
	}
}
