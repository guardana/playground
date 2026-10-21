package journal

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Version is the newest journal format this package writes and reads. A line
// that states no version is a call line of version 1; version 2 adds the auth
// line, which always states its version and its kind.
const Version = 2

// KindAuth is the kind an auth line states. A line that states no kind is a
// call line.
const KindAuth = "auth"

// Credential says what a request presented as its credential, never the value.
type Credential string

const (
	// CredentialNone means the request carried no Authorization header.
	CredentialNone Credential = "none"
	// CredentialRun means it carried exactly the run's bearer token.
	CredentialRun Credential = "run"
	// CredentialOther means it carried anything else: another token, another
	// scheme, a malformed header, or more than one.
	CredentialOther Credential = "other"
)

// Known reports whether c is one of the three credentials a writer records.
func (c Credential) Known() bool {
	return c == CredentialNone || c == CredentialRun || c == CredentialOther
}

// Session says what a request presented as its MCP session, never the id.
type Session string

const (
	// SessionNone means the request carried no session id.
	SessionNone Session = "none"
	// SessionKnown means it carried one id this server had issued.
	SessionKnown Session = "known"
	// SessionUnknown means it carried an id this server never issued, or more
	// than one.
	SessionUnknown Session = "unknown"
)

// Known reports whether s is one of the three sessions a writer records.
func (s Session) Known() bool {
	return s == SessionNone || s == SessionKnown || s == SessionUnknown
}

// Auth is one authentication decision a server made on a request to its MCP
// endpoint. Every field is an enumeration, a number or a JSON-RPC method name,
// so no field is free enough to hold a credential. Server is stamped by the
// writer and is ignored on the way in.
type Auth struct {
	OccurredAt time.Time  `json:"occurred_at"`
	Server     string     `json:"server"`
	RunID      string     `json:"run_id,omitempty"`
	Credential Credential `json:"credential"`
	Session    Session    `json:"session"`
	// Status is the HTTP status the server answered. It is a number where a
	// call line's status is a word, which is what makes a version 1 reader
	// refuse an auth line rather than read it as a call.
	Status int `json:"status"`
	// Method is the JSON-RPC method of the request, empty when the body held
	// none that could be read.
	Method string `json:"method,omitempty"`
}

// authLine is an auth line as it is written: version and kind first.
type authLine struct {
	Version int    `json:"version"`
	Kind    string `json:"kind"`
	Auth
}

// Bounds on a method name. MCP's longest segment is about a dozen letters; a
// 32-byte credential in hex or base64url is one segment of 43 characters or
// more, and almost never letters alone.
const (
	maxMethodBytes  = 64
	maxSegmentBytes = 24
)

// RecordAuth appends one auth line. It refuses a line a reader could not
// place, and any value outside the enumerations and the shape of a method
// name, so a credential cannot reach the journal through it. The refusal
// never repeats the value it refused.
func (w *Writer) RecordAuth(auth Auth) error {
	auth.Server = w.server
	if err := auth.validate(); err != nil {
		return err
	}
	line, err := json.Marshal(authLine{Version: Version, Kind: KindAuth, Auth: auth})
	if err != nil {
		return err
	}
	return w.append(line)
}

func (a Auth) validate() error {
	switch {
	case a.OccurredAt.IsZero():
		return fmt.Errorf("%w: auth line: no time", ErrInvalidEntry)
	case a.Server == "":
		return fmt.Errorf("%w: auth line: no server", ErrInvalidEntry)
	case !a.Credential.Known():
		return fmt.Errorf("%w: auth line: credential is not %s, %s or %s", ErrInvalidEntry, CredentialNone, CredentialRun, CredentialOther)
	case !a.Session.Known():
		return fmt.Errorf("%w: auth line: session is not %s, %s or %s", ErrInvalidEntry, SessionNone, SessionKnown, SessionUnknown)
	case a.Status < 100 || a.Status > 599:
		return fmt.Errorf("%w: auth line: status %d is not an HTTP status", ErrInvalidEntry, a.Status)
	case a.Method != "" && !ValidMethod(a.Method):
		return fmt.Errorf("%w: auth line: the method is not a JSON-RPC method name", ErrInvalidEntry)
	}
	return nil
}

// ValidMethod reports whether method has the shape of an MCP method name:
// segments of letters and underscores, each starting lowercase, joined by
// slashes, within the bounds above.
func ValidMethod(method string) bool {
	if method == "" || len(method) > maxMethodBytes {
		return false
	}
	for segment := range strings.SplitSeq(method, "/") {
		if !validSegment(segment) {
			return false
		}
	}
	return true
}

func validSegment(segment string) bool {
	if segment == "" || len(segment) > maxSegmentBytes || segment[0] < 'a' || segment[0] > 'z' {
		return false
	}
	for _, c := range []byte(segment) {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && c != '_' {
			return false
		}
	}
	return true
}
