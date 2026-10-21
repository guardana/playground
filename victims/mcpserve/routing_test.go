package mcpserve_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/victims/mcpserve"
)

const metadataPath = "/.well-known/oauth-protected-resource"

func answer(t *testing.T, handler http.Handler, method, path string) (int, string) {
	t.Helper()
	recorded := httptest.NewRecorder()
	handler.ServeHTTP(recorded, httptest.NewRequest(method, path, nil))
	body, err := io.ReadAll(recorded.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	return recorded.Code, string(body)
}

func probeServer() *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{Name: "probe", Version: "0"}, nil)
}

// A victim that states no routing serves what every victim served before the
// hook existed: /mcp bare, /healthz, and nothing else.
func TestNoRoutingServesOnlyMCPAndHealth(t *testing.T) {
	handler := mcpserve.NewHandler(probeServer(), func() bool { return true })
	if code, _ := answer(t, handler, http.MethodGet, metadataPath); code != http.StatusNotFound {
		t.Errorf("%s answered %d, want 404", metadataPath, code)
	}
	if code, _ := answer(t, handler, http.MethodGet, "/healthz"); code != http.StatusOK {
		t.Errorf("/healthz answered %d, want 200", code)
	}
}

func TestRoutingWrapsMCPAndAddsRoutes(t *testing.T) {
	teapot := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	}
	metadata := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "metadata") })
	handler, err := mcpserve.NewRoutedHandler(probeServer(), func() bool { return true }, mcpserve.Routing{
		Wrap:   teapot,
		Routes: map[string]http.Handler{"GET " + metadataPath: metadata},
	})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := answer(t, handler, http.MethodPost, "/mcp"); code != http.StatusTeapot {
		t.Errorf("/mcp answered %d, want the wrapper's 418", code)
	}
	if code, body := answer(t, handler, http.MethodGet, metadataPath); code != http.StatusOK || body != "metadata" {
		t.Errorf("%s answered %d %q, want the victim's route", metadataPath, code, body)
	}
	if code, _ := answer(t, handler, http.MethodGet, "/healthz"); code != http.StatusOK {
		t.Errorf("/healthz answered %d, want 200 and unwrapped", code)
	}
}

// /mcp and /healthz answer the same way on every victim. A method-specific
// pattern would not collide with them in the mux and would take them over.
func TestRoutingRefusesWhatWouldChangeTheFixedRoutes(t *testing.T) {
	ok := http.NotFoundHandler()
	tests := map[string]mcpserve.Routing{
		"GET on /mcp":           {Routes: map[string]http.Handler{"GET /mcp": ok}},
		"POST on /healthz":      {Routes: map[string]http.Handler{"POST /healthz": ok}},
		"a host on /mcp":        {Routes: map[string]http.Handler{"victim-vault/mcp": ok}},
		"a conflicting pattern": {Routes: map[string]http.Handler{"GET /{any}": ok}},
		"no path":               {Routes: map[string]http.Handler{"GET": ok}},
		"no handler":            {Routes: map[string]http.Handler{"GET " + metadataPath: nil}},
		"a wrap that drops it":  {Wrap: func(http.Handler) http.Handler { return nil }},
	}
	for name, routing := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("panicked: %v", recovered)
				}
			}()
			if _, err := mcpserve.NewRoutedHandler(probeServer(), func() bool { return true }, routing); err == nil {
				t.Error("accepted")
			}
		})
	}
}
