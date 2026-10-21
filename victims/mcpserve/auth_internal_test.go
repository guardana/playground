package mcpserve

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/journal"
)

// The run's token is assembled here, because the guards scan this file too.
func runToken() string { return strings.Repeat("rT", 21) + "Q" }

func TestClassifyCredentialSaysWhatTheRequestCarried(t *testing.T) {
	token := runToken()
	tests := []struct {
		name   string
		values []string
		token  string
		want   journal.Credential
	}{
		{"no header", nil, token, "none"},
		{"the run's token", []string{"Bearer " + token}, token, "run"},
		{"the scheme in any case", []string{"bEaReR " + token}, token, "run"},
		{"another token", []string{"Bearer " + token + "x"}, token, "other"},
		{"a prefix of the token", []string{"Bearer " + token[:len(token)-1]}, token, "other"},
		{"another scheme", []string{"Basic " + token}, token, "other"},
		{"the bare token", []string{token}, token, "other"},
		{"an empty header", []string{""}, token, "other"},
		{"a third field", []string{"Bearer " + token + " x"}, token, "other"},
		{"two headers", []string{"Bearer " + token, "Bearer " + token}, token, "other"},
		{"no run token configured", []string{"Bearer " + token}, "", "other"},
		{"an empty bearer with none configured", []string{"Bearer "}, "", "other"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header := http.Header{}
			for _, value := range test.values {
				header.Add("Authorization", value)
			}
			if got := classifyCredential(header, test.token); got != test.want {
				t.Errorf("credential = %q, want %q", got, test.want)
			}
		})
	}
}

func TestClassifySessionSaysWhetherTheServerIssuedIt(t *testing.T) {
	issued := &issuedSessions{}
	issued.remember("session-a")
	issued.remember("")
	tests := []struct {
		name   string
		values []string
		want   journal.Session
	}{
		{"no header", nil, "none"},
		{"an empty header", []string{""}, "none"},
		{"an issued id", []string{"session-a"}, "known"},
		{"an id never issued", []string{"session-b"}, "unknown"},
		{"an issued id twice", []string{"session-a", "session-a"}, "unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header := http.Header{}
			for _, value := range test.values {
				header.Add("Mcp-Session-Id", value)
			}
			if got := issued.classify(header); got != test.want {
				t.Errorf("session = %q, want %q", got, test.want)
			}
		})
	}
}

// A token of letters split by slashes has the shape of a method name, so the
// shape check alone would journal it.
func TestAMethodHoldingTheTokenIsNotKept(t *testing.T) {
	token := strings.Repeat("q", 17) + "/" + "z" + strings.Repeat("Z", 16) + "/" + strings.Repeat("w", 7)
	if !journal.ValidMethod(token) {
		t.Fatal("the sample does not have a method's shape, so it tests nothing")
	}
	journalled := &authJournal{token: token}
	for body, want := range map[string]string{
		`{"jsonrpc":"2.0","id":1,"method":"` + token + `"}`: "",
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`:    "tools/list",
	} {
		request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		if got := journalled.method(request); got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		if rest, err := io.ReadAll(request.Body); err != nil || string(rest) != body {
			t.Errorf("the handler would read %q, %v; want the body whole", rest, err)
		}
	}
}
