package compose

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/guardana/playground/internal/labspec"
)

// The proxy stands between the enforcer and one victim, so whoever can drive
// its API decides what the enforcer hears back. It gets what every lab service
// gets, one sealed network, and an API that listens only inside its container.
func TestTheChaosProxyIsHardenedAndDrivenFromInside(t *testing.T) {
	body, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Services map[string]struct {
			hardenedService
			Command []string `json:"command"`
		} `json:"services"`
	}
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("compose.yaml: %v", err)
	}
	proxy, declared := parsed.Services["toxiproxy-tools"]
	if !declared {
		t.Fatal("compose.yaml declares no toxiproxy-tools")
	}
	for what, held := range map[string]bool{
		"its image named by the pin":  proxy.Image == "${TOXIPROXY_IMAGE}",
		"uid 65532":                   proxy.User == "65532:65532",
		"a read-only root":            proxy.ReadOnly,
		"no capability":               slices.Equal(proxy.CapDrop, []string{"ALL"}),
		"no new privileges":           slices.Contains(proxy.SecurityOpt, "no-new-privileges:true"),
		"a tmpfs /tmp":                slices.Equal(proxy.Tmpfs, []string{"/tmp"}),
		"tool-net alone":              slices.Equal(proxy.Networks, []string{"tool-net"}),
		"its own profile alone":       slices.Equal(proxy.Profiles, []string{"chaos"}),
		"its API on loopback":         slices.Contains(proxy.Command, "-host=127.0.0.1"),
		"the proxies file, read-only": slices.Equal(proxy.Volumes, []any{"./toxiproxy/proxies.json:/etc/toxiproxy/proxies.json:ro"}),
	} {
		if !held {
			t.Errorf("toxiproxy-tools does not have %s: %+v", what, proxy)
		}
	}
}

// Each proxy forwards to one victim's MCP port, and no proxy listens where the
// API does: a proxy there would put the API on tool-net.
func TestEveryVictimHasOneProxyAndNothingElseDoes(t *testing.T) {
	body, err := os.ReadFile("toxiproxy/proxies.json")
	if err != nil {
		t.Fatal(err)
	}
	var proxies []struct {
		Name     string `json:"name"`
		Listen   string `json:"listen"`
		Upstream string `json:"upstream"`
	}
	if err := json.Unmarshal(body, &proxies); err != nil {
		t.Fatalf("toxiproxy/proxies.json: %v", err)
	}
	var named []string
	for _, proxy := range proxies {
		named = append(named, proxy.Name)
		if proxy.Upstream != proxy.Name+":8080" {
			t.Errorf("proxy %s forwards to %s, want %s:8080", proxy.Name, proxy.Upstream, proxy.Name)
		}
		if strings.HasSuffix(proxy.Listen, ":8474") || strings.HasSuffix(proxy.Listen, ":8080") {
			t.Errorf("proxy %s listens on %s, a port the API or a victim uses", proxy.Name, proxy.Listen)
		}
	}
	slices.Sort(named)
	want := slices.Sorted(slices.Values(labspec.Victims()))
	if !slices.Equal(named, want) {
		t.Errorf("proxies for %v, want %v", named, want)
	}
}
