package compose

import (
	"os"
	"slices"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/guardana/playground/internal/labspec"
)

type authDoubleService struct {
	hardenedService
	Command     []string          `json:"command"`
	Environment map[string]string `json:"environment"`
	Healthcheck struct {
		Test []string `json:"test"`
	} `json:"healthcheck"`
}

// The verifier fetches the double's metadata over tool-net, so the double is
// held to what a victim is held to, comes up with the verifier alone, and is
// told its persona by the variable the runner sets, defaulting to none.
func TestTheAuthDoubleIsHardenedOnToolNetWithTheVerifier(t *testing.T) {
	body, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Services map[string]authDoubleService `json:"services"`
	}
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("compose.yaml: %v", err)
	}
	double, declared := parsed.Services["auth-double"]
	if !declared {
		t.Fatal("compose.yaml declares no auth-double")
	}
	for what, held := range map[string]bool{
		"its image named by the lab prefix":     double.Image == "${LAB_IMAGE_PREFIX}-auth-double",
		"uid 65532":                             double.User == "65532:65532",
		"every capability dropped":              slices.Equal(double.CapDrop, []string{"ALL"}),
		"no capability added":                   len(double.CapAdd) == 0,
		"a read-only root":                      double.ReadOnly,
		"no new privileges":                     slices.Contains(double.SecurityOpt, "no-new-privileges:true"),
		"a tmpfs /tmp":                          slices.Equal(double.Tmpfs, []string{"/tmp"}),
		"no privileged mode":                    !double.Privileged,
		"no published port":                     len(double.Ports) == 0,
		"tool-net alone":                        slices.Equal(double.Networks, []string{"tool-net"}),
		"the verifier profile alone":            slices.Equal(double.Profiles, []string{"verifier"}),
		"its personas at /personas":             slices.Equal(double.Command, []string{"-personas=/personas"}),
		"the persona the runner names, or none": double.Environment[labspec.AuthPersonaVariable] == "${"+labspec.AuthPersonaVariable+":-}",
		"the run's id and reports directory": double.Environment["LAB_RUN_ID"] == "${LAB_RUN_ID:-manual}" &&
			double.Environment["LAB_REPORTS_DIR"] == "/reports",
		"a health check on its own listener": slices.Equal(double.Healthcheck.Test,
			[]string{"CMD", "/healthprobe", "http://127.0.0.1:8080/healthz"}),
	} {
		if !held {
			t.Errorf("auth-double does not have %s: %+v", what, double)
		}
	}
}
