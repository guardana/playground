package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// Whitespace, key order and an unknown member are the persona's to choose;
// the double reformats nothing.
const oddPersona = "{\n  \"token_endpoint\" : \"http://auth-double:8080/token\",\n\t\"issuer\":\"http://elsewhere.test\",\"x-lab\":[1, 2]\n}\n"

func TestAPersonaIsServedByteForByte(t *testing.T) {
	srv := startDouble(t, map[string]string{"odd": oddPersona}, "odd")
	status, body, header := srv.get(t, http.MethodGet, metadataPath, nil)
	if status != http.StatusOK || string(body) != oddPersona {
		t.Fatalf("served %d %q, want 200 and the persona as written", status, body)
	}
	if got := header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type is %q, want application/json", got)
	}
	lines, _ := srv.lines(t)
	if len(lines) != 1 || lines[0].Status != http.StatusOK || lines[0].Persona != "odd" ||
		lines[0].Method != "GET" || lines[0].Path != metadataPath || lines[0].RunID != testRunID ||
		lines[0].Server != "auth-double" {
		t.Fatalf("journal holds %+v, want one GET of the metadata answered 200 from persona odd", lines)
	}
}

// Without a persona there is no authorization server, and the double says so
// with a 404 on every metadata request rather than by stopping, so a run that
// named none still boots.
func TestWithoutALoadedPersonaEveryMetadataRequestIs404AndTheDoubleStaysUp(t *testing.T) {
	for name, persona := range map[string]string{
		"no persona named":             "",
		"a persona with no file":       "missing",
		"a persona that is not JSON":   "broken",
		"a name leaving the directory": "../outside",
	} {
		t.Run(name, func(t *testing.T) {
			// ../outside is written beside the personas directory, where only a
			// name that leaves the directory reaches it.
			srv := startDouble(t, map[string]string{
				"broken": `{"issuer": "http://auth-double:8080",`, "../outside": oddPersona,
			}, persona)
			for range 2 {
				if status, body, _ := srv.get(t, http.MethodGet, metadataPath, nil); status != http.StatusNotFound {
					t.Fatalf("metadata answered %d %q, want 404", status, body)
				}
			}
			if status, _, _ := srv.get(t, http.MethodGet, "/healthz", nil); status != http.StatusOK {
				t.Fatalf("/healthz answered %d after the 404s, want 200 from a double still up", status)
			}
			lines, _ := srv.lines(t)
			if len(lines) != 2 {
				t.Fatalf("journal holds %d lines, want the two metadata requests: %+v", len(lines), lines)
			}
			for _, entry := range lines {
				if entry.Status != http.StatusNotFound || entry.Persona != "" || entry.Path != metadataPath {
					t.Errorf("journal line %+v, want a metadata 404 with no persona", entry)
				}
			}
		})
	}
}

// The verifier presents the run's token to the victims and may present it
// here; the journal says it was there and never what it was.
func TestTheJournalRecordsHeaderPresenceAndNeverAValue(t *testing.T) {
	token, session := randomHex(t), randomHex(t)
	srv := startDouble(t, map[string]string{"odd": oddPersona}, "odd")
	srv.get(t, http.MethodGet, metadataPath, map[string]string{
		"Authorization": "Bearer " + token, "Mcp-Session-Id": session,
	})
	srv.get(t, http.MethodPost, "/token", map[string]string{"Authorization": "Basic " + token})
	srv.get(t, http.MethodGet, metadataPath, nil)

	lines, raw := srv.lines(t)
	want := []line{
		{Method: "GET", Path: metadataPath, Authorization: true, McpSessionID: true, Status: 200},
		{Method: "POST", Path: "/token", Authorization: true, McpSessionID: false, Status: 404},
		{Method: "GET", Path: metadataPath, Authorization: false, McpSessionID: false, Status: 200},
	}
	if len(lines) != len(want) {
		t.Fatalf("journal holds %d lines, want %d: %+v", len(lines), len(want), lines)
	}
	for i, entry := range lines {
		got := line{Method: entry.Method, Path: entry.Path, Authorization: entry.Authorization,
			McpSessionID: entry.McpSessionID, Status: entry.Status}
		if got != want[i] {
			t.Errorf("line %d is %+v, want %+v", i+1, got, want[i])
		}
	}
	for _, secret := range []string{token, session} {
		for start := 0; start+8 <= len(secret); start++ {
			if bytes.Contains(raw, []byte(secret[start:start+8])) {
				t.Fatalf("the journal holds part of a header value: %q", secret[start:start+8])
			}
		}
	}
}

// A credential can ride in a query string or under any header name; the
// journal says that one was there, and which header names came, never what
// they held.
func TestTheJournalRecordsAQueryAndHeaderNamesAndNeverAValue(t *testing.T) {
	secret := randomHex(t)
	srv := startDouble(t, map[string]string{"odd": oddPersona}, "odd")
	srv.get(t, http.MethodGet, metadataPath+"?access_token="+secret, map[string]string{"X-Api-Key": secret})
	srv.get(t, http.MethodGet, metadataPath, nil)
	lines, raw := srv.lines(t)
	if len(lines) != 2 {
		t.Fatalf("journal holds %d lines, want 2", len(lines))
	}
	if !lines[0].Query || !strings.Contains(lines[0].Headers, "X-Api-Key") {
		t.Errorf("line 1 has query %t and headers %q, want a query and X-Api-Key named", lines[0].Query, lines[0].Headers)
	}
	if lines[1].Query || strings.Contains(lines[1].Headers, "X-Api-Key") {
		t.Errorf("line 2 has query %t and headers %q, want neither", lines[1].Query, lines[1].Headers)
	}
	for start := 0; start+8 <= len(secret); start++ {
		if bytes.Contains(raw, []byte(secret[start:start+8])) {
			t.Fatalf("the journal holds part of a value: %q", secret[start:start+8])
		}
	}
}

// The health probe runs inside the container on loopback every two seconds;
// the same path asked from the network is a request like any other.
func TestOnlyTheLoopbackHealthProbeGoesUnjournalled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth-double.jsonl")
	d := &double{journal: openTestJournal(t, path), runID: testRunID, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, remote := range []string{"127.0.0.1:40000", "[::1]:40000", "10.0.0.7:40000"} {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.RemoteAddr = remote
		recorder := httptest.NewRecorder()
		d.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Errorf("/healthz from %s answered %d, want 200", remote, recorder.Code)
		}
	}
	lines, _ := readLines(t, path)
	if len(lines) != 1 || lines[0].Path != "/healthz" || lines[0].Status != http.StatusOK {
		t.Fatalf("journal holds %+v, want the one /healthz from 10.0.0.7", lines)
	}
}

func openTestJournal(t *testing.T, path string) *journalFile {
	t.Helper()
	j, err := openJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.close() })
	return j
}

func randomHex(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}
