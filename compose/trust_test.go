package compose

import (
	"os"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

type trusting struct {
	Services map[string]struct {
		Environment map[string]string `json:"environment"`
		Volumes     []any             `json:"volumes"`
	} `json:"services"`
}

// The enforcer trusts the decision point double's CA and the run's collector CA
// and no other root: without SSL_CERT_FILE and SSL_CERT_DIR Go would add the
// base image's whole bundle. Both, and the collector's key, are mounted
// read-only into the one service that reads them.
func TestTheEnforcerTrustsTheLabsTwoCAsAloneAndNothingWritesThem(t *testing.T) {
	body, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var parsed trusting
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("compose.yaml: %v", err)
	}
	env := parsed.Services["enforcer"].Environment
	if env["SSL_CERT_FILE"] != "/pki/ca.pem" || env["SSL_CERT_DIR"] != "/export-ca" {
		t.Errorf("the enforcer's roots are SSL_CERT_FILE=%q SSL_CERT_DIR=%q, want /pki/ca.pem and /export-ca",
			env["SSL_CERT_FILE"], env["SSL_CERT_DIR"])
	}
	for service, dirs := range map[string][]string{"enforcer": {"/pki", "/export-ca"}, "collector": {"/tls"}} {
		for _, target := range dirs {
			if !readOnlyAt(parsed.Services[service].Volumes, target) {
				t.Errorf("%s does not mount %s read-only", service, target)
			}
		}
	}
	for name, service := range parsed.Services {
		for _, volume := range service.Volumes {
			if name != "collector" && strings.HasSuffix(source(volume), "/collector-tls") {
				t.Errorf("%s mounts the collector's key", name)
			}
		}
	}
}

func readOnlyAt(volumes []any, target string) bool {
	for _, volume := range volumes {
		long, ok := volume.(map[string]any)
		if ok && long["target"] == target {
			readOnly, _ := long["read_only"].(bool)
			return readOnly
		}
	}
	return false
}
