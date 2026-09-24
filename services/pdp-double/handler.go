package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/guardana/playground/internal/journal"
)

const (
	maxQuestionBytes = 64 << 10
	// unreadableTool files a question the double could not read, since a
	// journal line has to name a tool.
	unreadableTool = "(unreadable)"
	// unroutedTool files a POST to a path the double does not evaluate on, so
	// a misconfigured endpoint is told apart from a plane that never asked.
	unroutedTool = "(unrouted)"
	// A line past journal.MaxLineBytes makes the whole journal unreadable.
	// These stay far under it even with every byte escaped to six.
	maxToolBytes   = 256
	maxDetailBytes = 2 << 10
)

// The answer bodies. Each is what one behaviour sends, byte for byte.
const (
	bodyAllow           = `{"decision":true}`
	bodyDeny            = `{"decision":false}`
	bodyAllowObligation = `{"decision":true,"context":{"obligations":[{"id":"notify-owner"}]}}`
	bodyMalformed       = `{"decision":true,`
	bodyExtraMember     = `{"decision":true,"advice":[]}`
	bodyUnscripted      = `{"decision":false,"context":{"reason_admin":{"en":"no rule in the pdp-double script matched this request"}}}`
)

// double answers AuthZEN evaluations as its script says and journals every
// one it receives before answering it.
type double struct {
	script         *script
	journal        *journal.Writer
	runID          string
	hold           time.Duration
	evaluationPath string
	discoveryPath  string
	metadata       []byte
	logger         *slog.Logger
}

func newDouble(s settings, sc *script, w *journal.Writer, logger *slog.Logger) (*double, error) {
	metadata, err := json.Marshal(struct {
		PolicyDecisionPoint      string `json:"policy_decision_point"`
		AccessEvaluationEndpoint string `json:"access_evaluation_endpoint"`
	}{s.identifier, s.endpoint()})
	if err != nil {
		return nil, err
	}
	return &double{
		script: sc, journal: w, runID: s.runID, hold: s.hold,
		evaluationPath: s.evaluationPath(), discoveryPath: s.discoveryPath(),
		metadata: metadata, logger: logger,
	}, nil
}

func (d *double) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == d.evaluationPath && r.Method == http.MethodPost:
		d.evaluate(w, r)
	case r.URL.Path == d.discoveryPath && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(d.metadata)
	case r.URL.Path == "/healthz" && r.Method == http.MethodGet:
		_, _ = io.WriteString(w, "ok\n")
	default:
		if r.Method == http.MethodPost {
			_ = d.record(unroutedTool, journal.Refused, r.Method+" "+r.URL.EscapedPath())
		}
		http.NotFound(w, r)
	}
}

func (d *double) evaluate(w http.ResponseWriter, r *http.Request) {
	q, err := readQuestion(r.Body)
	if err != nil {
		if d.record(unreadableTool, journal.Refused, "unreadable request") {
			http.Error(w, "unreadable evaluation request", http.StatusBadRequest)
		} else {
			http.Error(w, "journal unavailable", http.StatusServiceUnavailable)
		}
		return
	}
	chosen := d.script.answerFor(q)
	// A question the journal did not take is one a scenario could not see
	// was asked, so it is never answered with a decision.
	if !d.record(q.Action.Name, journal.Served, string(chosen)) {
		http.Error(w, "journal unavailable", http.StatusServiceUnavailable)
		return
	}
	d.respond(w, r, chosen)
}

func readQuestion(body io.Reader) (question, error) {
	text, err := io.ReadAll(io.LimitReader(body, maxQuestionBytes+1))
	if err != nil {
		return question{}, err
	}
	if len(text) > maxQuestionBytes {
		return question{}, errors.New("evaluation request too long")
	}
	var q question
	if err := json.Unmarshal(text, &q); err != nil {
		return question{}, err
	}
	if q.Action.Name == "" {
		return question{}, errors.New("evaluation request names no action")
	}
	return q, nil
}

func (d *double) record(tool string, status journal.Status, detail string) bool {
	tool = cut(tool, maxToolBytes)
	err := d.journal.Record(journal.Entry{
		OccurredAt: time.Now().UTC(), Tool: tool, RunID: d.runID, Status: status, Detail: cut(detail, maxDetailBytes),
	})
	if err != nil {
		d.logger.Error("journal write failed", "tool", tool, "error", err)
		return false
	}
	return true
}

// cut keeps at most limit bytes of s, ending on a whole character, and says
// how long s was when it had to cut.
func cut(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	end := limit
	for end > limit-utf8.UTFMax && !utf8.RuneStart(s[end]) {
		end--
	}
	return fmt.Sprintf("%s [cut from %d bytes]", s[:end], len(s))
}

func (d *double) respond(w http.ResponseWriter, r *http.Request, chosen answer) {
	requestID := r.Header.Get("X-Request-ID")
	switch chosen {
	case answerTimeout:
		if heldToBound(r.Context(), d.hold) {
			http.Error(w, "held past the double's own bound", http.StatusServiceUnavailable)
		}
	case answerStatus500:
		http.Error(w, "scripted failure", http.StatusInternalServerError)
	case answerNoEcho:
		writeAnswer(w, "", bodyAllow)
	default:
		writeAnswer(w, requestID, bodyFor(chosen))
	}
}

func bodyFor(chosen answer) string {
	switch chosen {
	case answerAllow:
		return bodyAllow
	case answerDeny:
		return bodyDeny
	case answerAllowObligation:
		return bodyAllowObligation
	case answerMalformed:
		return bodyMalformed
	case answerExtraMember:
		return bodyExtraMember
	}
	return bodyUnscripted
}

func writeAnswer(w http.ResponseWriter, echo, body string) {
	w.Header().Set("Content-Type", "application/json")
	if echo != "" {
		w.Header().Set("X-Request-ID", echo)
	}
	_, _ = io.WriteString(w, body)
}

// heldToBound waits until the caller gives up or the bound passes, and
// reports whether the bound passed with the caller still waiting.
func heldToBound(ctx context.Context, bound time.Duration) bool {
	timer := time.NewTimer(bound)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
