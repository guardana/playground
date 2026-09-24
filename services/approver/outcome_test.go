package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

const approveAnything = "schema_version: 1\nrules:\n  - {match: {}, answer: approve, approver_id: a}\n"

// withoutPlane is a listing as the command prints it once the plane that held
// the directory is gone.
func withoutPlane(t *testing.T, listing string) string {
	t.Helper()
	out := strings.Replace(listing, planeHolds+"\n", planeAbsent+"\n", 1)
	if out == listing {
		t.Fatal("the listing names no plane holding the directory, so nothing was changed")
	}
	return out
}

// answeredIn is the listing with one record answered as the command writes it.
func answeredIn(t *testing.T, listing, id, state, by string) string {
	t.Helper()
	head, rest, ok := strings.Cut(listing, "approval "+id+"\n")
	if !ok {
		t.Fatalf("the listing holds no approval %s", id)
	}
	block, tail, _ := strings.Cut(rest, "\n\n")
	var lines []string
	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(line, "  state ") {
			line = fmt.Sprintf("  %-14s %s", "state", state)
		}
		lines = append(lines, line)
		if strings.HasPrefix(line, "  expires ") {
			lines = append(lines, fmt.Sprintf("  %-14s %s", "answered by", by), fmt.Sprintf("  %-14s %s", "answered at", "2026-09-24T08:02:01Z"))
		}
	}
	out := head + "approval " + id + "\n" + strings.Join(lines, "\n")
	if tail != "" {
		out += "\n\n" + tail
	}
	return out
}

func healthOf(err error) int {
	var hl health
	hl.set(err)
	rec := httptest.NewRecorder()
	hl.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	return rec.Code
}

// An answer written while no plane holds the directory exits zero and reaches
// no call: the hold did not survive the plane. So nothing is answered, the fact
// is journalled once per approval, and the health check fails while it lasts.
func TestNothingIsAnsweredWhileNoPlaneHoldsTheDirectory(t *testing.T) {
	h := newHarness(t, "every-state.txt", approveAnything, 10*time.Second)
	held := fixture(t, "every-state.txt", h.lab.dir)
	h.lab.setListing(withoutPlane(t, held))
	for i := 0; i < 2; i++ {
		err := h.tick(t, epoch().Add(time.Duration(i)*time.Second))
		if !errors.Is(err, errNoPlane) {
			t.Fatalf("tick %d: err = %v, want errNoPlane", i, err)
		}
		if got := healthOf(err); got != http.StatusServiceUnavailable {
			t.Errorf("tick %d: health %d while no plane holds the directory, want 503", i, got)
		}
	}
	h.lab.setListing(held)
	if err := h.tick(t, epoch().Add(2*time.Second)); err != nil {
		t.Fatalf("with a plane back: %v", err)
	}
	if got := h.answers(t); len(got) != 0 {
		t.Errorf("answered %q, records no plane held", got)
	}
	expectLines(t, h.lines(t), [][3]string{
		{"no-plane", "served", "approval=STALE"},
		{"no-plane", "served", "approval=WAITING"},
	})
}

// A record whose projection is missing or describes another approval shows
// nothing a rule could have meant, so only a rule naming that case answers it.
func TestAnUnreadableRecordIsAnsweredOnlyByARuleThatNamesIt(t *testing.T) {
	for _, name := range []string{"no-view.txt", "other-view.txt", "broken-view.txt"} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, name, approveAnything, 10*time.Second)
			if err := h.tick(t, epoch()); err != nil {
				t.Fatal(err)
			}
			for _, c := range h.answers(t) {
				if c[len(c)-1] == "WAITING" {
					t.Errorf("a catch-all answered the unreadable record: %q", c)
				}
			}
			lines := h.lines(t)
			if last := lines[len(lines)-1]; last != [3]string{"leave", "served", "approval=WAITING unmatched unreadable"} {
				t.Errorf("last line %q, want the unreadable record left and named", last)
			}
		})
	}
	h := newHarness(t, "no-view.txt",
		"schema_version: 1\nrules:\n  - {match: {unreadable: true}, answer: reject, approver_id: a}\n  - {match: {}, answer: approve, approver_id: a}\n",
		10*time.Second)
	if err := h.tick(t, epoch()); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"approvals", "reject", "--approver-id", "a", h.lab.dir, "WAITING"}}
	if got := h.answers(t); !reflect.DeepEqual(got, want) {
		t.Errorf("answers = %q, want %q", got, want)
	}
}

// A command killed at its deadline may already have written the answer, so
// the line says the outcome is unknown and the next listing says what the
// record shows.
func TestAKilledAnswerIsUnknownUntilTheRecordSays(t *testing.T) {
	cases := map[string]struct {
		after func(t *testing.T, listing string) string
		want  [3]string
	}{
		"written": {
			func(t *testing.T, l string) string {
				return answeredIn(t, l, "WAITING", "APPROVAL_STATE_APPROVED", "a")
			},
			[3]string{"approve", "served", "approval=WAITING record=APPROVAL_STATE_APPROVED/pending answered_by=a"},
		},
		"not written": {
			func(_ *testing.T, l string) string { return l },
			[3]string{"approve", "refused", "approval=WAITING record=APPROVAL_STATE_PENDING/pending"},
		},
		"answered by another": {
			func(t *testing.T, l string) string {
				return answeredIn(t, l, "WAITING", "APPROVAL_STATE_APPROVED", "duty-officer")
			},
			[3]string{"unknown", "served", "approval=WAITING answer=approve record=APPROVAL_STATE_APPROVED/pending answered_by=duty-officer"},
		},
		"gone": {
			func(t *testing.T, l string) string { return dropped(t, l, "WAITING") },
			[3]string{"unknown", "served", "approval=WAITING answer=approve record=absent"},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, "mixed.txt", "schema_version: 1\nrules:\n  - {match: {action: refund}, answer: approve, approver_id: a}\n", 800*time.Millisecond)
			h.lab.config.HangAnswers = true
			h.lab.save()
			started := time.Now()
			if err := h.tick(t, epoch()); err != nil {
				t.Fatal(err)
			}
			if took := time.Since(started); took > 10*time.Second {
				t.Errorf("the hung command held the approver %v", took)
			}
			h.lab.setListing(c.after(t, fixture(t, "mixed.txt", h.lab.dir)))
			for i := 1; i < 3; i++ {
				if err := h.tick(t, epoch().Add(time.Duration(i)*time.Second)); err != nil {
					t.Fatal(err)
				}
			}
			if got := h.answers(t); len(got) != 1 {
				t.Errorf("answers = %q, want exactly one attempt", got)
			}
			expectLines(t, h.lines(t), [][3]string{
				{"leave", "served", "approval=PURGE unmatched"},
				{"unknown", "served", "approval=WAITING answer=approve"},
				c.want,
			})
		})
	}
}

// dropped is the listing without one record, as the command prints it once
// the record is pruned.
func dropped(t *testing.T, listing, id string) string {
	t.Helper()
	head, rest, ok := strings.Cut(listing, "\napproval "+id+"\n")
	if !ok {
		t.Fatalf("the listing holds no approval %s", id)
	}
	_, tail, _ := strings.Cut(rest, "\n\n")
	out := strings.TrimSuffix(head, "\n") + "\n"
	if tail != "" {
		out = head + "\n" + tail
	}
	return strings.Replace(out, "3 records", "2 records", 1)
}

// cancelOnceAnswering cancels when the fake command has received an answer,
// which it then hangs on.
func cancelOnceAnswering(ctx context.Context, cancel context.CancelFunc, calls string) {
	for ctx.Err() == nil {
		body, _ := os.ReadFile(calls) // #nosec G304 -- the test's own calls log.
		if strings.Contains(string(body), `"approve"`) {
			cancel()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Shutdown cancels the command mid-answer: that answer is unknown, nothing
// else is answered, and one last listing settles what the record shows.
func TestShutdownAnswersNothingMoreAndSettlesWhatWasCutShort(t *testing.T) {
	h := newHarness(t, "mixed.txt", approveAnything, 10*time.Second)
	h.lab.config.HangAnswers = true
	h.lab.save()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	go cancelOnceAnswering(ctx, cancel, h.lab.config.Calls)
	if err := h.a.tick(ctx, epoch()); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want the cancellation", err)
	}
	settle, done := context.WithTimeout(context.Background(), 20*time.Second)
	defer done()
	if err := h.a.settle(settle); err != nil {
		t.Fatal(err)
	}
	if got := h.answers(t); len(got) != 1 || got[0][len(got[0])-1] != "PURGE" {
		t.Errorf("answers = %q, want only the one cut short", got)
	}
	expectLines(t, h.lines(t), [][3]string{
		{"unknown", "served", "approval=PURGE answer=approve"},
		{"approve", "refused", "approval=PURGE record=APPROVAL_STATE_PENDING/pending"},
	})
}
