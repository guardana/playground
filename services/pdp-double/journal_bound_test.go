package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/guardana/playground/internal/journal"
)

// A line past journal.MaxLineBytes makes the whole journal unreadable, so the
// bounds below are literals well under it, not the double's own constants.
const (
	toolBound   = 1 << 10
	detailBound = 8 << 10
)

func TestAnOversizedPathLeavesTheJournalReadable(t *testing.T) {
	h := startDouble(t, scriptAnswering("allow"), 10*time.Second)
	// Each %3C decodes to "<", which JSON writes as six bytes: past the
	// journal's line limit if the path were journalled decoded and whole.
	h.evaluation = "/" + strings.Repeat("%3C", 200_000)
	_, resp, err := h.post(t, 10*time.Second, "req-0010", questionFor("crm.refund", "order", "ord-1", "user", "user-7", "req-0010"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404", resp.StatusCode)
	}
	entries := h.entries(t)
	if len(entries) != 1 || entries[0].Tool != "(unrouted)" || entries[0].Status != journal.Refused {
		t.Fatalf("journal %+v, want one refused (unrouted) line", entries)
	}
	detail := entries[0].Detail
	sent := len("POST ") + len(h.evaluation)
	if len(detail) > detailBound || !strings.HasPrefix(detail, "POST /%3C%3C") ||
		!strings.HasSuffix(detail, fmt.Sprintf("[cut from %d bytes]", sent)) {
		t.Fatalf("detail is %d bytes, want at most %d, the path as sent, saying it was cut from %d bytes: %.80q...",
			len(detail), detailBound, sent, detail)
	}
}

func TestAnOversizedActionNameIsCutInTheJournal(t *testing.T) {
	h := startDouble(t, scriptAnswering("allow"), 10*time.Second)
	action := strings.Repeat("<", 60_000)
	if got := h.ask(t, 5*time.Second, "req-0011", questionFor(action, "order", "ord-1", "user", "user-7", "req-0011")); got != "PDP_DENY" {
		t.Fatalf("control reads %s, want PDP_DENY", got)
	}
	entries := h.entries(t)
	if len(entries) != 1 || entries[0].Status != journal.Served || entries[0].Detail != "unscripted" {
		t.Fatalf("journal %+v, want one served line answered unscripted", entries)
	}
	tool := entries[0].Tool
	if len(tool) > toolBound || !strings.HasPrefix(tool, "<<<<") || !strings.HasSuffix(tool, "[cut from 60000 bytes]") {
		t.Fatalf("tool is %d bytes, want at most %d, saying it was cut from 60000 bytes: %.80q...", len(tool), toolBound, tool)
	}
}

func TestACutKeepsWholeCharacters(t *testing.T) {
	h := startDouble(t, scriptAnswering("allow"), 10*time.Second)
	// Four offsets against a four-byte character: whatever the bound, most
	// of these cuts land inside one.
	for _, lead := range []string{"", "a", "aa", "aaa"} {
		action := lead + strings.Repeat("\U0001D11E", 2_000)
		h.ask(t, 5*time.Second, "req-0012", questionFor(action, "order", "ord-1", "user", "user-7", "req-0012"))
	}
	for i, entry := range h.entries(t) {
		want := fmt.Sprintf("[cut from %d bytes]", 8_000+i)
		if !utf8.ValidString(entry.Tool) || strings.ContainsRune(entry.Tool, utf8.RuneError) || !strings.HasSuffix(entry.Tool, want) {
			t.Errorf("the cut split a character or did not say %s: %.80q...", want, entry.Tool)
		}
	}
}
