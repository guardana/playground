package journal_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/journal"
)

func record(t *testing.T, entry journal.Entry) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "victim-pay.jsonl")
	w, err := journal.Open(path, "victim-pay")
	if err != nil {
		t.Fatal(err)
	}
	if entry.OccurredAt.IsZero() {
		entry.OccurredAt = time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	}
	recordErr := w.Record(entry)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return path, recordErr
}

func TestAnEffectIsReadBackMemberForMember(t *testing.T) {
	effect := journal.Effect{
		"amount": journal.Integer(5000), "currency": journal.String("EUR"), "captured": journal.Bool(false),
	}
	path, err := record(t, journal.Entry{Tool: "pay.charge", Status: journal.Served, Effect: effect})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"effect":{"amount":5000,"captured":false,"currency":"EUR"}`; !strings.Contains(string(body), want) {
		t.Errorf("line %s does not hold %s", body, want)
	}
	entries, err := journal.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := entries[0].Effect
	if !got.Equal(effect) {
		t.Errorf("read back %s, want %s", got, effect)
	}
	if got["amount"].Equal(journal.String("5000")) || got["captured"].Equal(journal.Integer(0)) {
		t.Error("a value of one kind equals a value of another")
	}
}

// Each bound is tested at the value past it and at the value on it, so a test
// that passes cannot be one the limit was never reached in.
func TestRecordRefusesAnEffectItCannotCarry(t *testing.T) {
	members := func(n int) journal.Effect {
		effect := journal.Effect{}
		for i := range n {
			effect[fmt.Sprintf("m%d", i)] = journal.Integer(int64(i))
		}
		return effect
	}
	cases := []struct {
		name   string
		status journal.Status
		effect journal.Effect
		ok     bool
	}{
		{"on a refused line", journal.Refused, journal.Effect{"amount": journal.Integer(1)}, false},
		{"no member", journal.Served, journal.Effect{}, false},
		{"sixteen members", journal.Served, members(16), true},
		{"seventeen members", journal.Served, members(17), false},
		{"an empty name", journal.Served, journal.Effect{"": journal.Integer(1)}, false},
		{"a 64-byte name", journal.Served, journal.Effect{strings.Repeat("n", 64): journal.Integer(1)}, true},
		{"a 65-byte name", journal.Served, journal.Effect{strings.Repeat("n", 65): journal.Integer(1)}, false},
		{"a 256-byte string", journal.Served, journal.Effect{"s": journal.String(strings.Repeat("s", 256))}, true},
		{"a 257-byte string", journal.Served, journal.Effect{"s": journal.String(strings.Repeat("s", 257))}, false},
		{"a string not UTF-8", journal.Served, journal.Effect{"s": journal.String("\xff")}, false},
		{"2^53-1", journal.Served, journal.Effect{"n": journal.Integer(1<<53 - 1)}, true},
		{"2^53", journal.Served, journal.Effect{"n": journal.Integer(1 << 53)}, false},
		{"-(2^53-1)", journal.Served, journal.Effect{"n": journal.Integer(-(1<<53 - 1))}, true},
		{"-(2^53)", journal.Served, journal.Effect{"n": journal.Integer(-(1 << 53))}, false},
		{"no value", journal.Served, journal.Effect{"n": {}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := record(t, journal.Entry{Tool: "pay.charge", Status: c.status, Effect: c.effect})
			if c.ok && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !c.ok && !errors.Is(err, journal.ErrInvalidEntry) {
				t.Fatalf("err = %v, want ErrInvalidEntry", err)
			}
		})
	}
}

// The reader holds a line to the same rules, reading the raw number: a value
// float64 would round into a match is refused, never compared.
func TestReadFileRefusesAnEffectItCannotReadExactly(t *testing.T) {
	cases := map[string]bool{
		`{"amount":9007199254740991}`:  true,
		`{"amount":-9007199254740991}`: true,
		`{"amount":9007199254740992}`:  false,
		`{"amount":9007199254740993}`:  false,
		`{"amount":5000.0}`:            false,
		`{"amount":5e3}`:               false,
		`{"amount":null}`:              false,
		`{"amount":{"value":1}}`:       false,
		`{"amount":[1]}`:               false,
		`{}`:                           false,
		`null`:                         true,
	}
	for effect, ok := range cases {
		t.Run(effect, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "victim-pay.jsonl")
			line := `{"occurred_at":"2026-10-02T09:00:00Z","server":"victim-pay","tool":"pay.charge",` +
				`"status":"served","effect":` + effect + "}\n"
			if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := journal.ReadFile(path)
			if ok && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !ok && err == nil {
				t.Fatal("read")
			}
		})
	}
}

func TestAValueDecodesAsTheKindItIsWritten(t *testing.T) {
	for text, want := range map[string]journal.Value{
		`"5000"`: journal.String("5000"), `5000`: journal.Integer(5000), `-1`: journal.Integer(-1),
		`true`: journal.Bool(true), ` false `: journal.Bool(false),
	} {
		var got journal.Value
		if err := json.Unmarshal([]byte(text), &got); err != nil {
			t.Errorf("%s: %v", text, err)
			continue
		}
		if !got.Equal(want) {
			t.Errorf("%s decoded as %s, want %s", text, got, want)
		}
	}
}
