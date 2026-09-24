package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/journal"
)

func TestEachScriptedAnswerIsReadByControlAsItsCode(t *testing.T) {
	for _, tc := range []struct {
		answer string
		want   string
	}{
		{"allow", "PDP_ALLOW"},
		{"deny", "PDP_DENY"},
		{"allow_obligation", "OBLIGATION_NOT_UNDERSTOOD"},
		{"status_500", "PDP_UNAVAILABLE"},
		{"no_echo", "PDP_ANSWER_REFUSED"},
		{"malformed", "PDP_ANSWER_REFUSED"},
		{"extra_member", "PDP_ANSWER_REFUSED"},
	} {
		t.Run(tc.answer, func(t *testing.T) {
			h := startDouble(t, scriptAnswering(tc.answer), 10*time.Second)
			body := questionFor("crm.refund", "order", "ord-1", "user", "user-7", "req-0001")
			if got := h.ask(t, 5*time.Second, "req-0001", body); got != tc.want {
				t.Fatalf("control reads %s, want %s", got, tc.want)
			}
			assertOneEntry(t, h, "crm.refund", journal.Served, tc.answer)
		})
	}
}

// exchange is one raw answer as control's client receives it.
type exchange struct {
	echo, contentType []string
	body              []byte
}

func (e exchange) echoed() bool { return len(e.echo) == 1 && e.echo[0] == "req-0002" }

func onlyEchoMissing(e exchange) bool {
	return len(e.echo) == 0 && declaresJSON(e.contentType) && readDecision(e.body) == codeAllow
}

func onlyUnparsable(e exchange) bool {
	_, err := strictObject(e.body)
	return err != nil && e.echoed() && declaresJSON(e.contentType)
}

func onlyAnExtraMember(e exchange) bool {
	top, err := strictObject(e.body)
	return err == nil && e.echoed() && declaresJSON(e.contentType) &&
		string(top["decision"]) == "true" && len(top) == 2 && top["context"] == nil
}

func TestRefusedAnswersCarryOnlyTheirOwnDefect(t *testing.T) {
	for _, tc := range []struct {
		answer string
		only   func(exchange) bool
	}{
		{"no_echo", onlyEchoMissing},
		{"malformed", onlyUnparsable},
		{"extra_member", onlyAnExtraMember},
	} {
		t.Run(tc.answer, func(t *testing.T) {
			h := startDouble(t, scriptAnswering(tc.answer), 10*time.Second)
			_, resp, err := h.post(t, 5*time.Second, "req-0002", questionFor("crm.refund", "order", "ord-1", "user", "user-7", "req-0002"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			body, err := io.ReadAll(resp.Body)
			if err != nil || resp.StatusCode != http.StatusOK {
				t.Fatalf("status %d, read error %v", resp.StatusCode, err)
			}
			got := exchange{echo: resp.Header.Values("X-Request-ID"), contentType: resp.Header.Values("Content-Type"), body: body}
			if !tc.only(got) {
				t.Fatalf("the answer carries another defect besides its own: echo %q, type %q, body %s", got.echo, got.contentType, got.body)
			}
		})
	}
}

func TestAQuestionNoRuleMatchesIsDenied(t *testing.T) {
	for name, script := range map[string]string{
		"another action scripted": scriptAnswering("allow"),
		"no rules":                "schema_version: 1\nrules: []\n",
	} {
		t.Run(name, func(t *testing.T) {
			h := startDouble(t, script, 10*time.Second)
			got, body := h.askReading(t, 5*time.Second, "req-0003", questionFor("crm.lookup", "customer", "cus-1", "user", "user-7", "req-0003"))
			if got != "PDP_DENY" {
				t.Fatalf("control reads %s, want PDP_DENY", got)
			}
			if !strings.Contains(string(body), "no rule in the pdp-double script matched") {
				t.Fatalf("the denial does not say nothing was scripted: %s", body)
			}
			assertOneEntry(t, h, "crm.lookup", journal.Served, "unscripted")
		})
	}
}

func TestTimeoutHoldsPastTheCallersDeadline(t *testing.T) {
	h := startDouble(t, scriptAnswering("timeout"), 10*time.Second)
	started := time.Now()
	got := h.ask(t, 300*time.Millisecond, "req-0004", questionFor("crm.refund", "order", "ord-1", "user", "user-7", "req-0004"))
	if got != "PDP_TIMEOUT" {
		t.Fatalf("control reads %s, want PDP_TIMEOUT", got)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("the ask took %v, past the caller's deadline by far", elapsed)
	}
	assertOneEntry(t, h, "crm.refund", journal.Served, "timeout")
}

func TestTimeoutIsBoundedByTheDoublesOwnHold(t *testing.T) {
	h := startDouble(t, scriptAnswering("timeout"), 400*time.Millisecond)
	started := time.Now()
	got := h.ask(t, 10*time.Second, "req-0005", questionFor("crm.refund", "order", "ord-1", "user", "user-7", "req-0005"))
	if got != "PDP_UNAVAILABLE" {
		t.Fatalf("control reads %s, want PDP_UNAVAILABLE once the double's hold ran out", got)
	}
	if elapsed := time.Since(started); elapsed < 400*time.Millisecond || elapsed > 5*time.Second {
		t.Fatalf("the ask took %v, want the double's hold of 400ms", elapsed)
	}
}

func TestRulesMatchOnWhatTheRequestCarries(t *testing.T) {
	h := startDouble(t, `schema_version: 1
rules:
  - match: {action: crm.refund, resource_type: order, resource_id: ord-1, subject_id: user-7}
    answer: allow
  - match: {action: crm.refund, subject_type: service}
    answer: extra_member
  - match: {action: crm.refund}
    answer: deny
`, 10*time.Second)
	for _, q := range [][5]string{
		{"crm.refund", "order", "ord-1", "user", "user-7"},
		{"crm.refund", "order", "ord-2", "user", "user-7"},
		{"crm.refund", "invoice", "ord-1", "user", "user-7"},
		{"crm.refund", "order", "ord-1", "user", "user-8"},
		{"crm.refund", "order", "ord-9", "service", "svc-1"},
		{"crm.export", "order", "ord-1", "user", "user-7"},
	} {
		h.ask(t, 5*time.Second, "req-0006", questionFor(q[0], q[1], q[2], q[3], q[4], "req-0006"))
	}
	var got []string
	for _, entry := range h.entries(t) {
		got = append(got, entry.Detail)
	}
	want := "allow deny deny deny extra_member unscripted"
	if strings.Join(got, " ") != want {
		t.Fatalf("answers applied %q, want %q", strings.Join(got, " "), want)
	}
}

func TestAnUnreadableQuestionIsJournalledAsRefused(t *testing.T) {
	h := startDouble(t, scriptAnswering("allow"), 10*time.Second)
	if got := h.ask(t, 5*time.Second, "req-0007", `{"action":`); got != "PDP_UNAVAILABLE" {
		t.Fatalf("control reads %s, want PDP_UNAVAILABLE", got)
	}
	assertOneEntry(t, h, "(unreadable)", journal.Refused, "unreadable request")
}

func assertOneEntry(t *testing.T, h *harness, tool string, status journal.Status, detail string) {
	t.Helper()
	entries := h.entries(t)
	if len(entries) != 1 {
		t.Fatalf("journal has %d entries, want 1: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.Server != "pdp-double" || e.Tool != tool || e.Status != status || e.Detail != detail || e.RunID != "run-pdp-1" {
		t.Fatalf("journal entry %+v, want server pdp-double, tool %s, status %s, detail %s, run run-pdp-1", e, tool, status, detail)
	}
}
