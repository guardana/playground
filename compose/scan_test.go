package compose

import (
	"os"
	"reflect"
	"slices"
	"testing"

	"sigs.k8s.io/yaml"
)

type scanService struct {
	hardenedService
	NetworkMode string `json:"network_mode"`
	WorkingDir  string `json:"working_dir"`
	DependsOn   any    `json:"depends_on"`
}

func readScan(t *testing.T) map[string]scanService {
	t.Helper()
	body, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Services map[string]scanService `json:"services"`
	}
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("compose.yaml: %v", err)
	}
	return parsed.Services
}

// A scan reads files a scenario planted as hostile and needs no peer, so its
// container has no network interface but loopback, and sees its step's inputs
// alone, read-only. The verifier reports paths relative to where it runs, so
// the working directory is part of what a scenario's expected scope names.
func TestTheScanVerifierHasNoNetworkAndReadsItsStepAlone(t *testing.T) {
	scanner, declared := readScan(t)["scan-verifier"]
	if !declared {
		t.Fatal("compose.yaml declares no scan-verifier")
	}
	inputs := map[string]any{
		"type":      "bind",
		"source":    "${LAB_RUN_HOST_DIR:-/LAB_RUN_HOST_DIR-is-unset}/verifier/${LAB_VERIFIER_DIR:-LAB_VERIFIER_DIR-is-unset}/in",
		"target":    "/lab-run/in",
		"read_only": true,
		"bind":      map[string]any{"create_host_path": false},
	}
	for what, held := range map[string]bool{
		"its image named by the pins":         scanner.Image == "${VERIFIER_IMAGE}:${VERIFIER_VERSION}",
		"no image pulled in its name":         scanner.PullPolicy == "never",
		"no image built by compose":           scanner.Build == nil,
		"uid 65532":                           scanner.User == "65532:65532",
		"a read-only root":                    scanner.ReadOnly,
		"no capability":                       slices.Equal(scanner.CapDrop, []string{"ALL"}),
		"no capability added":                 len(scanner.CapAdd) == 0,
		"no privileged mode":                  !scanner.Privileged,
		"no new privileges":                   slices.Contains(scanner.SecurityOpt, "no-new-privileges:true"),
		"a tmpfs /tmp":                        slices.Equal(scanner.Tmpfs, []string{"/tmp"}),
		"no published port":                   len(scanner.Ports) == 0,
		"no network":                          scanner.NetworkMode == "none" && len(scanner.Networks) == 0,
		"no service it waits on":              scanner.DependsOn == nil,
		"its own profile alone":               slices.Equal(scanner.Profiles, []string{"scan"}),
		"its step's inputs as working dir":    scanner.WorkingDir == "/lab-run/in",
		"its step's inputs, read-only, alone": reflect.DeepEqual(scanner.Volumes, []any{inputs}),
	} {
		if !held {
			t.Errorf("scan-verifier does not have %s: %+v", what, scanner)
		}
	}
}

// A service with no profile boots under every profile, so a scan would run
// beside it; one that lists `scan` would boot beside the scanner. Either puts
// a peer next to the files a scan step plants.
func TestTheScanProfileBootsTheScanVerifierAlone(t *testing.T) {
	var inScan []string
	for name, service := range readScan(t) {
		if len(service.Profiles) == 0 {
			t.Errorf("%s declares no profile, so it boots with every profile, scan among them", name)
		}
		if slices.Contains(service.Profiles, "scan") {
			inScan = append(inScan, name)
		}
	}
	if !slices.Equal(inScan, []string{"scan-verifier"}) {
		t.Errorf("the scan profile boots %v, want scan-verifier alone", inScan)
	}
}
