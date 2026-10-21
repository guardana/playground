package mcpserve

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/guardana/playground/internal/journal"
)

const (
	sessionHeader = "Mcp-Session-Id"
	// methodPeekBytes is how much of a request body is read for its method.
	// A body past it is passed on whole and journalled with no method.
	methodPeekBytes = 64 << 10
)

// JournalAuth wraps a victim's MCP handler, and the authentication in front of
// it, so that every request it answers leaves an auth line: whether it carried
// token, the run's bearer token, another credential or none; whether it
// carried a session id this server issued; the status it was answered with;
// and its JSON-RPC method. No value is written, only what kind it was.
//
// It must be the outermost wrapper: it learns the sessions the server issues
// from the responses that pass through it. A decision it cannot journal is
// answered 500 instead, because an answer with no line would be a request the
// run's record says never arrived.
func (r *Recorder) JournalAuth(token string, next http.Handler) http.Handler {
	return &authJournal{recorder: r, token: token, next: next, issued: &issuedSessions{}}
}

type authJournal struct {
	recorder *Recorder
	token    string
	next     http.Handler
	issued   *issuedSessions
}

func (a *authJournal) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	credential := classifyCredential(req.Header, a.token)
	session := a.issued.classify(req.Header)
	method := a.method(req)
	writer := &decidingWriter{ResponseWriter: w, decide: func(status int, header http.Header) error {
		a.issued.remember(header.Get(sessionHeader))
		return a.recorder.recordAuth(credential, session, status, method)
	}}
	a.next.ServeHTTP(writer, req)
	writer.decided()
}

// classifyCredential reads the Authorization header the way go-sdk's bearer
// check does: one header, two fields, the scheme in any case.
func classifyCredential(header http.Header, token string) journal.Credential {
	values := header.Values("Authorization")
	if len(values) == 0 {
		return journal.CredentialNone
	}
	if len(values) > 1 || token == "" {
		return journal.CredentialOther
	}
	fields := strings.Fields(values[0])
	if len(fields) == 2 && strings.EqualFold(fields[0], "bearer") &&
		subtle.ConstantTimeCompare([]byte(fields[1]), []byte(token)) == 1 {
		return journal.CredentialRun
	}
	return journal.CredentialOther
}

// method reads the JSON-RPC method from a POST body and puts the body back
// for the handler. A method that could hold the token is not kept.
func (a *authJournal) method(req *http.Request) string {
	if req.Method != http.MethodPost || req.Body == nil {
		return ""
	}
	peek, err := io.ReadAll(io.LimitReader(req.Body, methodPeekBytes+1))
	req.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(peek), req.Body), req.Body}
	if err != nil || len(peek) > methodPeekBytes {
		return ""
	}
	var message struct {
		Method string `json:"method"`
	}
	if json.Unmarshal(peek, &message) != nil || !journal.ValidMethod(message.Method) {
		return ""
	}
	if a.token != "" && strings.Contains(message.Method, a.token) {
		return ""
	}
	return message.Method
}

// issuedSessions holds the session ids this server answered with.
type issuedSessions struct {
	mu  sync.Mutex
	ids map[string]struct{}
}

func (s *issuedSessions) remember(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ids == nil {
		s.ids = map[string]struct{}{}
	}
	s.ids[id] = struct{}{}
}

// classify reads the session header as go-sdk does, where an empty id is no
// id; more than one is never a session the server issued.
func (s *issuedSessions) classify(header http.Header) journal.Session {
	values := header.Values(sessionHeader)
	if len(values) == 0 || (len(values) == 1 && values[0] == "") {
		return journal.SessionNone
	}
	s.mu.Lock()
	_, issued := s.ids[values[0]]
	s.mu.Unlock()
	if len(values) == 1 && issued {
		return journal.SessionKnown
	}
	return journal.SessionUnknown
}

func (r *Recorder) recordAuth(credential journal.Credential, session journal.Session, status int, method string) error {
	return r.writer.RecordAuth(journal.Auth{
		OccurredAt: time.Now().UTC(),
		RunID:      r.runID,
		Credential: credential,
		Session:    session,
		Status:     status,
		Method:     method,
	})
}

var errUnrecorded = errors.New("mcpserve: the auth decision was not journalled")

// decidingWriter journals the decision when the status is written, before the
// header reaches the client, so a decision is never answered without a line.
type decidingWriter struct {
	http.ResponseWriter
	decide func(status int, header http.Header) error
	done   bool
	failed bool
}

func (d *decidingWriter) WriteHeader(status int) {
	if d.done {
		d.ResponseWriter.WriteHeader(status)
		return
	}
	d.done = true
	if err := d.decide(status, d.Header()); err != nil {
		d.failed = true
		d.Header().Del(sessionHeader)
		http.Error(d.ResponseWriter, errUnrecorded.Error(), http.StatusInternalServerError)
		return
	}
	d.ResponseWriter.WriteHeader(status)
}

func (d *decidingWriter) Write(body []byte) (int, error) {
	d.decided()
	if d.failed {
		return 0, errUnrecorded
	}
	return d.ResponseWriter.Write(body)
}

// Flush is what an event stream calls; it sends the header first.
func (d *decidingWriter) Flush() {
	d.decided()
	if d.failed {
		return
	}
	_ = http.NewResponseController(d.ResponseWriter).Flush()
}

// Unwrap lets http.ResponseController reach the connection's deadlines.
func (d *decidingWriter) Unwrap() http.ResponseWriter { return d.ResponseWriter }

// decided writes the implicit 200 a handler that wrote nothing answers with.
func (d *decidingWriter) decided() {
	if !d.done {
		d.WriteHeader(http.StatusOK)
	}
}
