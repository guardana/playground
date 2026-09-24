package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Compose waits on this endpoint before it starts anything that depends on the
// gateway. A dependency that waits on a service being merely started is how a
// runner ends up asserting against a service that has not finished starting.
func TestHealthzAnswersOnceTheServerIsBuilt(t *testing.T) {
	empty := mcp.NewServer(&mcp.Implementation{Name: "stub-gateway", Version: stubVersion}, nil)
	server := httptest.NewServer(newMux(empty))
	t.Cleanup(server.Close)

	response, err := server.Client().Get(server.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Errorf("GET /healthz answered %d", response.StatusCode)
	}
}

func TestTheGatewayServesNothingButItsTwoPaths(t *testing.T) {
	empty := mcp.NewServer(&mcp.Implementation{Name: "stub-gateway", Version: stubVersion}, nil)
	server := httptest.NewServer(newMux(empty))
	t.Cleanup(server.Close)

	response, err := server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("GET / answered %d, want 404", response.StatusCode)
	}
}
