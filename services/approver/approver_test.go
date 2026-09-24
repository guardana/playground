package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/guardana/playground/internal/journal"
)

func epoch() time.Time { return time.Date(2026, 9, 24, 8, 2, 0, 0, time.UTC) }

// harness is an approver wired to a fake lab, with its journal kept where the
// test can read it back.
type harness struct {
	lab     *fakeLab
	a       *approver
	journal string
}

func newHarness(t *testing.T, listingFixture, scriptBody string, timeout time.Duration) *harness {
	t.Helper()
	lab := newFakeLab(t, listingFixture)
	s, err := parseScript([]byte(scriptBody))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "approver.jsonl")
	w, err := journal.Open(path, serverName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	a := newApprover(lab.dir, lab.commander(timeout), s, w, "run-7", slog.New(slog.NewTextHandler(io.Discard, nil)))
	return &harness{lab: lab, a: a, journal: path}
}

func (h *harness) tick(t *testing.T, at time.Time) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return h.a.tick(ctx, at)
}

// lines is the journal as tool, status and detail, the three fields a scenario
// grades from.
func (h *harness) lines(t *testing.T) [][3]string {
	t.Helper()
	entries, err := journal.ReadFile(h.journal)
	if err != nil {
		t.Fatal(err)
	}
	var out [][3]string
	for _, e := range entries {
		if e.Server != "approver" || e.RunID != "run-7" {
			t.Errorf("a line from server %q run %q", e.Server, e.RunID)
		}
		out = append(out, [3]string{e.Tool, string(e.Status), e.Detail})
	}
	return out
}

func (h *harness) answers(t *testing.T) [][]string {
	t.Helper()
	var out [][]string
	for _, c := range h.lab.calls() {
		if len(c) < 2 || c[1] != "list" {
			out = append(out, c)
		}
	}
	return out
}

func expectLines(t *testing.T, got, want [][3]string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("journal = %q, want %q", got, want)
	}
}

const approveRefunds = "schema_version: 1\nrules:\n  - {match: {action: refund}, answer: approve, approver_id: lab-approver, reason: scripted yes}\n"

// Only the two records still waiting are answered; the approved, consumed and
// closed ones are left to what already became of them.
func TestApproveAnswersEveryWaitingRecordOnce(t *testing.T) {
	h := newHarness(t, "every-state.txt", approveRefunds, 10*time.Second)
	for i := 0; i < 3; i++ {
		if err := h.tick(t, epoch().Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	d := h.lab.dir
	want := [][]string{
		{"approvals", "approve", "--approver-id", "lab-approver", "--reason", "scripted yes", d, "STALE"},
		{"approvals", "approve", "--approver-id", "lab-approver", "--reason", "scripted yes", d, "WAITING"},
	}
	if got := h.answers(t); !reflect.DeepEqual(got, want) {
		t.Errorf("answers = %q, want %q", got, want)
	}
	if got := h.lab.calls()[0]; !reflect.DeepEqual(got, []string{"approvals", "list", d}) {
		t.Errorf("first call = %q, want the listing", got)
	}
	expectLines(t, h.lines(t), [][3]string{
		{"approve", "served", "approval=STALE exit=0"},
		{"approve", "served", "approval=WAITING exit=0"},
	})
}

func TestRejectAnswersWithTheCommandsOwnVerb(t *testing.T) {
	h := newHarness(t, "mixed.txt",
		"schema_version: 1\nrules:\n  - {match: {effect_class: EFFECT_CLASS_DELETE}, answer: reject, approver_id: lab-approver}\n  - {answer: leave}\n",
		10*time.Second)
	if err := h.tick(t, epoch()); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"approvals", "reject", "--approver-id", "lab-approver", h.lab.dir, "PURGE"}}
	if got := h.answers(t); !reflect.DeepEqual(got, want) {
		t.Errorf("answers = %q, want %q", got, want)
	}
	expectLines(t, h.lines(t), [][3]string{
		{"reject", "served", "approval=PURGE exit=0"},
		{"leave", "served", "approval=WAITING rule=2"},
	})
}

func TestLeaveRunsNothingAndIsRecordedOnce(t *testing.T) {
	h := newHarness(t, "mixed.txt", "schema_version: 1\nrules:\n  - {match: {action: refund}, answer: leave}\n", 10*time.Second)
	for i := 0; i < 3; i++ {
		if err := h.tick(t, epoch().Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if got := h.answers(t); len(got) != 0 {
		t.Errorf("leave ran %q", got)
	}
	expectLines(t, h.lines(t), [][3]string{
		{"leave", "served", "approval=PURGE unmatched"},
		{"leave", "served", "approval=WAITING rule=1"},
	})
}

// The delay counts from the first listing that showed the approval, so the
// answer lands on the first tick at or past it and on no earlier one.
func TestDelayHoldsTheAnswerUntilItIsDue(t *testing.T) {
	h := newHarness(t, "mixed.txt",
		"schema_version: 1\nrules:\n  - {match: {action: refund}, answer: approve, approver_id: a, delay: 2s}\n  - {answer: leave}\n",
		10*time.Second)
	for _, at := range []time.Duration{0, time.Second, 2*time.Second - time.Millisecond} {
		if err := h.tick(t, epoch().Add(at)); err != nil {
			t.Fatal(err)
		}
		if got := h.answers(t); len(got) != 0 {
			t.Fatalf("at +%v the approver answered %q before the delay", at, got)
		}
	}
	if err := h.tick(t, epoch().Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"approvals", "approve", "--approver-id", "a", h.lab.dir, "WAITING"}}
	if got := h.answers(t); !reflect.DeepEqual(got, want) {
		t.Errorf("answers = %q, want %q", got, want)
	}
	expectLines(t, h.lines(t), [][3]string{
		{"leave", "served", "approval=PURGE rule=2"},
		{"wait", "served", "approval=WAITING rule=1 delay=2s"},
		{"approve", "served", "approval=WAITING exit=0"},
	})
}

// A delayed answer the approval did not live to see still leaves the line of
// its first sighting, naming the rule that was waiting.
func TestADelayedAnswerThatNeverRanIsJournalled(t *testing.T) {
	h := newHarness(t, "mixed.txt",
		"schema_version: 1\nrules:\n  - {match: {action: refund}, answer: approve, approver_id: a, delay: 1m}\n  - {answer: leave}\n",
		10*time.Second)
	for i := 0; i < 3; i++ {
		if err := h.tick(t, epoch().Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if got := h.answers(t); len(got) != 0 {
		t.Errorf("answered %q before the delay", got)
	}
	expectLines(t, h.lines(t), [][3]string{
		{"leave", "served", "approval=PURGE rule=2"},
		{"wait", "served", "approval=WAITING rule=1 delay=1m"},
	})
}

// A projection that will not decode leaves a rule nothing to match on, so the
// record is left alone rather than answered by a rule that names an action,
// and the line says why.
func TestAnApprovalNoRuleMatchesIsLeftAndSaysSo(t *testing.T) {
	h := newHarness(t, "broken-view.txt", approveRefunds, 10*time.Second)
	if err := h.tick(t, epoch()); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"approvals", "approve", "--approver-id", "lab-approver", "--reason", "scripted yes", h.lab.dir, "STALE"}}
	if got := h.answers(t); !reflect.DeepEqual(got, want) {
		t.Errorf("answers = %q, want %q", got, want)
	}
	expectLines(t, h.lines(t), [][3]string{
		{"approve", "served", "approval=STALE exit=0"},
		{"leave", "served", "approval=WAITING unmatched unreadable"},
	})
}

// A refusal is journalled with the command's own exit status and not retried:
// the refusal is the enforcer's answer, and a retry would bury it.
func TestARefusingCommandIsJournalledAndNotRetried(t *testing.T) {
	h := newHarness(t, "mixed.txt", approveRefunds, 10*time.Second)
	h.lab.config.AnswerExit["WAITING"] = 1
	h.lab.save()
	for i := 0; i < 2; i++ {
		if err := h.tick(t, epoch().Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if got := h.answers(t); len(got) != 1 {
		t.Errorf("answers = %q, want exactly one attempt", got)
	}
	expectLines(t, h.lines(t), [][3]string{
		{"leave", "served", "approval=PURGE unmatched"},
		{"approve", "refused", "approval=WAITING exit=1"},
	})
}
