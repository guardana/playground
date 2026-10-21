package check_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/runner/check"
)

func doubleLine(run, path string, authorization, session bool) string {
	return servedLine(run, path, authorization, session, 200, "honest")
}

func servedLine(run, path string, authorization, session bool, status int, persona string) string {
	b := func(v bool) string {
		if v {
			return "true"
		}
		return "false"
	}
	return `{"occurred_at":"2026-10-22T10:00:00Z","server":"auth-double","run_id":"` + run + `","method":"GET","path":"` + path +
		`","authorization":` + b(authorization) + `,"mcp_session_id":` + b(session) + `,"status":` + strconv.Itoa(status) + named(persona) + `}`
}

const metadataPath = "/.well-known/oauth-authorization-server"

func gradeDouble(t *testing.T, lines ...string) assertion.Result {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "auth-double"), 0o750); err != nil {
		t.Fatal(err)
	}
	if lines != nil {
		body := strings.Join(lines, "\n") + "\n"
		if err := os.WriteFile(filepath.Join(dir, "auth-double", "auth-double.jsonl"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	results, err := check.AuthDouble{JournalDir: dir, Persona: "honest"}.Run(context.Background(), assertion.Records{RunID: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Check != "auth-double/unauthenticated" {
		t.Fatalf("results = %+v, want auth-double/unauthenticated alone", results)
	}
	return results[0]
}

func TestTheDiscoveryDocumentIsFetchedWithNoCredential(t *testing.T) {
	for name, test := range map[string]struct {
		lines   []string
		outcome assertion.Outcome
	}{
		"fetched bare":                  {[]string{doubleLine("run-1", metadataPath, false, false)}, assertion.Pass},
		"fetched with the bearer token": {[]string{doubleLine("run-1", metadataPath, true, false)}, assertion.Fail},
		"fetched with a session":        {[]string{doubleLine("run-1", metadataPath, false, true)}, assertion.Fail},
		"another path with the token": {[]string{doubleLine("run-1", metadataPath, false, false),
			doubleLine("run-1", "/token", true, false)}, assertion.Fail},
		"fetched only in another run": {[]string{doubleLine("run-0", metadataPath, false, false)}, assertion.Indeterminate},
		"never asked":                 {[]string{}, assertion.Indeterminate},
		"a line of another format":    {[]string{`{"run_id":"run-1","path":"` + metadataPath + `","bearer":"x"}`}, assertion.Fail},
		"no journal":                  {nil, assertion.Fail},
		// The double answering 404 is a persona the run never handed it: the
		// verifier then read no metadata, whatever it reported.
		"a credential in the query": {[]string{strings.Replace(doubleLine("run-1", metadataPath, false, false),
			`"status":200`, `"query":true,"headers":"Accept","status":200`, 1)}, assertion.Fail},
		"a key under another header": {[]string{strings.Replace(doubleLine("run-1", metadataPath, false, false),
			`"status":200`, `"query":false,"headers":"Accept,X-Api-Key","status":200`, 1)}, assertion.Fail},
		"ordinary headers": {[]string{strings.Replace(doubleLine("run-1", metadataPath, false, false),
			`"status":200`, `"query":false,"headers":"Accept,Accept-Encoding,User-Agent","status":200`, 1)}, assertion.Pass},
		"asked and answered 404": {[]string{servedLine("run-1", metadataPath, false, false, 404, "")}, assertion.Fail},
		"served another persona": {[]string{servedLine("run-1", metadataPath, false, false, 200, "no-pkce")}, assertion.Fail},
	} {
		t.Run(name, func(t *testing.T) {
			if got := gradeDouble(t, test.lines...); got.Outcome != test.outcome {
				t.Errorf("auth-double/unauthenticated = %s (%q: %s), want %s", got.Outcome, got.Got, got.Detail, test.outcome)
			}
		})
	}
}

func named(persona string) string {
	if persona == "" {
		return ""
	}
	return `,"persona":"` + persona + `"`
}
