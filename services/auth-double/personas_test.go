package main

import (
	"encoding/json"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

const labPersonas = "../../config/auth"

// The lab's personas load and are served as the files hold them, and each
// differs from the honest one by its named lie alone.
func TestTheLabPersonasAreServedAsWritten(t *testing.T) {
	honest := readPersona(t, "honest")
	for what, want := range map[string]any{
		"issuer":                                         "http://auth-double:8080",
		"authorization_endpoint":                         "http://auth-double:8080/authorize",
		"token_endpoint":                                 "http://auth-double:8080/token", // #nosec G101 -- an endpoint URL, not a credential.
		"response_types_supported":                       []any{"code"},
		"code_challenge_methods_supported":               []any{"S256"},
		"authorization_response_iss_parameter_supported": true,
		"scopes_supported":                               []any{"vault.read", "vault.write"},
	} {
		if !reflect.DeepEqual(honest[what], want) {
			t.Errorf("honest %s is %v, want %v", what, honest[what], want)
		}
	}
	if len(honest) != 7 {
		t.Errorf("honest holds %v, want the seven members above", slices.Sorted(maps.Keys(honest)))
	}
	noPKCE := readPersona(t, "no-pkce")
	delete(honest, "code_challenge_methods_supported")
	if !reflect.DeepEqual(noPKCE, honest) {
		t.Errorf("no-pkce is %v, want honest without code_challenge_methods_supported", noPKCE)
	}
}

func readPersona(t *testing.T, name string) map[string]any {
	t.Helper()
	written, err := os.ReadFile(filepath.Join(labPersonas, name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := startDouble(t, map[string]string{name: string(written)}, name)
	status, body, _ := srv.get(t, http.MethodGet, metadataPath, nil)
	if status != http.StatusOK || string(body) != string(written) {
		t.Fatalf("%s served %d %q, want 200 and the file as written", name, status, body)
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return parsed
}
