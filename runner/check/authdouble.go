package check

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/internal/runfile"
)

// authMetadataPath is the document the verifier fetches from an authorization
// server a protected resource names (RFC 8414).
const authMetadataPath = "/.well-known/oauth-authorization-server"

// AuthDouble grades the authorization-server double's own record of what the
// verifier sent it. The verifier documents its discovery requests as
// unauthenticated GETs, and the double is another origin than the server under
// test, so none of its requests may carry a credential or a session.
type AuthDouble struct {
	JournalDir string
	// Persona is the persona the scenario named, which the double has to
	// have served.
	Persona string
}

// ID names the check in a report.
func (AuthDouble) ID() string { return "auth-double" }

// doubleLine mirrors one line the double writes, field for field: a field it
// starts writing shows up as a line this check refuses, never as silence.
type doubleLine struct {
	OccurredAt    time.Time `json:"occurred_at"`
	Server        string    `json:"server"`
	RunID         string    `json:"run_id"`
	Method        string    `json:"method"`
	Path          string    `json:"path"`
	Authorization bool      `json:"authorization"`
	McpSessionID  bool      `json:"mcp_session_id"`
	Query         bool      `json:"query"`
	Headers       string    `json:"headers"`
	Status        int       `json:"status"`
	Persona       string    `json:"persona,omitempty"`
}

// Run grades this run's lines of the double's journal.
func (a AuthDouble) Run(_ context.Context, records assertion.Records) ([]assertion.Result, error) {
	return []assertion.Result{a.grade(records)}, nil
}

func (a AuthDouble) grade(records assertion.Records) assertion.Result {
	source := journal.File(a.JournalDir, labspec.AuthDoubleJournal)
	result := assertion.Result{
		Check:  "auth-double/unauthenticated",
		Want:   "persona " + a.Persona + " served in this run, every request bare of a credential and a session",
		Source: source,
	}
	lines, err := readDoubleLines(source)
	if err != nil {
		result.Outcome, result.Got, result.Detail = assertion.Fail, "no journal read", err.Error()
		return result
	}
	fetched, served, carried := a.tally(lines, records.RunID)
	switch {
	case len(carried) > 0:
		result.Outcome, result.Got = assertion.Fail, "carried: "+strings.Join(carried, "; ")
	case fetched == 0:
		result.Outcome, result.Got = assertion.Indeterminate, "no metadata request in this run"
	case served == 0:
		// The double answered without the persona the run named: what the
		// verifier read, if anything, is not what the scenario is about.
		result.Outcome = assertion.Fail
		result.Got = fmt.Sprintf("%d metadata request(s), none answered with persona %s", fetched, a.Persona)
	default:
		result.Outcome, result.Got = assertion.Pass, fmt.Sprintf("%d bare metadata request(s), %d served %s", fetched, served, a.Persona)
	}
	return result
}

// tally counts this run's metadata requests and those answered with the
// persona, and names each request that carried a credential or a session.
func (a AuthDouble) tally(lines []doubleLine, runID string) (fetched, served int, carried []string) {
	for _, line := range lines {
		if line.RunID != runID {
			continue
		}
		if line.Method == "GET" && line.Path == authMetadataPath {
			fetched++
			if line.Status == 200 && line.Persona == a.Persona {
				served++
			}
		}
		if line.Authorization || line.McpSessionID || line.Query || credentialHeader(line.Headers) {
			carried = append(carried, fmt.Sprintf("%s %s (authorization %t, session %t, query %t, headers %s)",
				line.Method, line.Path, line.Authorization, line.McpSessionID, line.Query, line.Headers))
		}
	}
	return fetched, served, carried
}

// credentialHeader reports whether any header name the double recorded is one
// a credential or a session travels under.
func credentialHeader(names string) bool {
	for _, name := range strings.Split(strings.ToLower(names), ",") {
		for _, marker := range []string{"auth", "token", "key", "cookie", "session", "secret", "credential"} {
			if strings.Contains(name, marker) {
				return true
			}
		}
	}
	return false
}

// readDoubleLines reads the journal as a regular file within the journal
// bound, and refuses a line it does not know rather than skip it.
func readDoubleLines(path string) ([]doubleLine, error) {
	body, err := runfile.ReadRegular(path, journal.MaxFileBytes)
	if err != nil {
		return nil, err
	}
	var lines []doubleLine
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64<<10), journal.MaxLineBytes)
	for number := 1; scanner.Scan(); number++ {
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		var line doubleLine
		if err := decoder.Decode(&line); err != nil {
			return nil, fmt.Errorf("line %d: %w", number, err)
		}
		lines = append(lines, line)
	}
	return lines, scanner.Err()
}
