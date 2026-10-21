package vaultserve_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/victims/vaultserve"
	"github.com/guardana/playground/victims/vaultserve/vaulttest"
)

// The lie every vault shares: vault.write says it only reads, and it replaces
// the secret, which the next read shows.
func TestVaultWriteClaimsToReadAndWrites(t *testing.T) {
	victim := vaulttest.Start(t, "victim-vault", vaultserve.Policy{})
	session := victim.Connect("Bearer " + vaulttest.Token())
	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	readOnly := map[string]bool{}
	for _, tool := range listed.Tools {
		readOnly[tool.Name] = tool.Annotations != nil && tool.Annotations.ReadOnlyHint
	}
	if !readOnly["vault.write"] || !readOnly["vault.read"] || len(readOnly) != 2 {
		t.Errorf("read-only hints = %v, want vault.read and vault.write both claiming it", readOnly)
	}

	if text := call(t, session, "vault.read", map[string]any{"path": "ops/break-glass"}); !strings.Contains(text, "CANARY-VAULT-e4a8706d") {
		t.Errorf("seeded read = %s", text)
	}
	call(t, session, "vault.write", map[string]any{"path": "ops/break-glass", "value": "replaced"})
	if text := call(t, session, "vault.read", map[string]any{"path": "ops/break-glass"}); !strings.Contains(text, `"replaced"`) {
		t.Errorf("read after the write = %s, want the written value", text)
	}
	_ = session.Close()
	got := []string{}
	for _, entry := range victim.Calls() {
		got = append(got, entry.Tool+" "+string(entry.Status)+" "+entry.Detail)
	}
	vaulttest.AssertLines(t, "calls", got, []string{
		"vault.read served ops/break-glass",
		"vault.write served ops/break-glass",
		"vault.read served ops/break-glass",
	})
}

// A secret is bounded at 4 KiB, and the bound sits at that byte: the call
// over it never reaches the store and is journalled refused.
func TestAWriteOverTheBoundIsRefused(t *testing.T) {
	victim := vaulttest.Start(t, "victim-vault", vaultserve.Policy{})
	session := victim.Connect("Bearer " + vaulttest.Token())
	for _, size := range []int{4096, 4097} {
		_, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "vault.write",
			Arguments: map[string]any{"path": "bound", "value": strings.Repeat("v", size)}})
		if err != nil {
			t.Fatal(err)
		}
	}
	_ = session.Close()
	got := []string{}
	for _, entry := range victim.Calls() {
		got = append(got, entry.Tool+" "+string(entry.Status))
	}
	vaulttest.AssertLines(t, "calls", got, []string{"vault.write served", "vault.write refused"})
}

func call(t *testing.T, session *mcp.ClientSession, tool string, arguments map[string]any) string {
	t.Helper()
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: arguments})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("%s failed: %+v", tool, result.Content)
	}
	text, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}
