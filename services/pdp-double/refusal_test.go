package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/guardana/playground/internal/journal"
)

func TestNoAnswerLeavesWhenTheJournalRefusesTheLine(t *testing.T) {
	for name, body := range map[string]string{
		"scripted allow":  questionFor("crm.refund", "order", "ord-1", "user", "user-7", "req-0008"),
		"unreadable body": `{"action":`,
	} {
		t.Run(name, func(t *testing.T) {
			h := startDouble(t, scriptAnswering("allow"), 10*time.Second)
			if err := h.writer.Close(); err != nil {
				t.Fatal(err)
			}
			ctx, resp, err := h.post(t, 5*time.Second, "req-0008", body)
			if err != nil {
				t.Fatal(err)
			}
			status := resp.StatusCode
			if got := controlReads(ctx, resp, nil, "req-0008"); got != "PDP_UNAVAILABLE" || status != http.StatusServiceUnavailable {
				t.Fatalf("control reads %s from status %d, want PDP_UNAVAILABLE from 503", got, status)
			}
			if entries := h.entries(t); len(entries) != 0 {
				t.Fatalf("a closed journal took %d entries: %+v", len(entries), entries)
			}
		})
	}
}

func TestAPostOutsideTheEvaluationPathIsJournalledAsRefused(t *testing.T) {
	for _, path := range []string{"/access/v1/evaluation", "/pdp/access/v1/evaluations", "/healthz"} {
		t.Run(path, func(t *testing.T) {
			h := startDouble(t, scriptAnswering("allow"), 10*time.Second)
			h.evaluation = path
			_, resp, err := h.post(t, 5*time.Second, "req-0009", questionFor("crm.refund", "order", "ord-1", "user", "user-7", "req-0009"))
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status %d, want 404", resp.StatusCode)
			}
			assertOneEntry(t, h, "(unrouted)", journal.Refused, "POST "+path)
		})
	}
}
