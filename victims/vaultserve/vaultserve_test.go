package vaultserve_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/guardana/playground/victims/mcpserve"
	"github.com/guardana/playground/victims/vaultserve"
	"github.com/guardana/playground/victims/vaultserve/vaulttest"
)

func lies() map[string]vaultserve.Policy {
	return map[string]vaultserve.Policy{
		"victim-vault":             {},
		"victim-vault-anytoken":    {AcceptAnyToken: true},
		"victim-vault-sessionauth": {SessionStandsIn: true},
	}
}

// The challenge names where the metadata is and the scope to ask for, and the
// metadata declares this vault's own origin and the double as its issuer.
func TestTheChallengeAndTheMetadataNameThisVault(t *testing.T) {
	for name, policy := range lies() {
		t.Run(name, func(t *testing.T) {
			victim := vaulttest.Start(t, name, policy)
			answer := victim.Post("", "", vaulttest.Initialize)
			want := `Bearer resource_metadata="http://` + name + `:8080/.well-known/oauth-protected-resource", scope="vault.read"`
			if answer.Status != http.StatusUnauthorized || answer.Challenge != want {
				t.Errorf("no token answered %d %q, want 401 %q", answer.Status, answer.Challenge, want)
			}
			metadata := fetchMetadata(t, victim.URL)
			if metadata.Resource != "http://"+name+":8080" ||
				!slices.Equal(metadata.AuthorizationServers, []string{"http://auth-double:8080"}) ||
				!slices.Equal(metadata.ScopesSupported, []string{"vault.read", "vault.write"}) {
				t.Errorf("metadata = %+v", metadata)
			}
		})
	}
}

type resourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported"`
}

func fetchMetadata(t *testing.T, base string) resourceMetadata {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/.well-known/oauth-protected-resource", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("metadata answered %d", response.StatusCode)
	}
	var metadata resourceMetadata
	if err := json.NewDecoder(response.Body).Decode(&metadata); err != nil {
		t.Fatal(err)
	}
	return metadata
}

// What a gated server lists depends on who asked, so no shared cache may keep
// it. go-sdk's own default is public.
func TestTheToolListingIsPrivate(t *testing.T) {
	session := vaulttest.Start(t, "victim-vault", vaultserve.Policy{}).Connect("Bearer " + vaulttest.Token())
	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if listed.CacheScope != "private" {
		t.Errorf("cacheScope = %q, want private", listed.CacheScope)
	}
}

// A vault with no token would gate on nothing, so it does not start, and it
// refuses before it has opened a journal the runner would read as a server
// that answered nothing.
func TestAVaultWithoutTheTokenRefusesToStart(t *testing.T) {
	reports := t.TempDir()
	t.Setenv("LAB_LISTEN", "127.0.0.1:0")
	t.Setenv("LAB_SERVER_NAME", "victim-vault")
	t.Setenv("LAB_RUN_ID", "run-1")
	t.Setenv("LAB_REPORTS_DIR", reports)
	t.Setenv(vaultserve.TokenVariable, "")
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	if err := vaultserve.Run(ctx, "victim-vault", vaultserve.Policy{}); !errors.Is(err, vaultserve.ErrNoToken) {
		t.Errorf("Run = %v, want %v", err, vaultserve.ErrNoToken)
	}
	if _, err := os.Stat(filepath.Join(reports, "run-1")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a vault that refused to start left %v behind", err)
	}
}

func TestBuildRefusesAnEmptyToken(t *testing.T) {
	config := mcpserve.Config{ServerName: "victim-vault", RunID: "run-1", ReportsDir: t.TempDir()}
	recorder, err := mcpserve.OpenJournal(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close() })
	if _, _, err := vaultserve.Build(recorder, "victim-vault", "", vaultserve.Policy{}); !errors.Is(err, vaultserve.ErrNoToken) {
		t.Errorf("Build = %v, want %v", err, vaultserve.ErrNoToken)
	}
}
