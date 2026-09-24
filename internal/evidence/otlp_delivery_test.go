package evidence_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/evidence"
)

// Export is at-least-once: a batch the collector took but did not acknowledge
// in time is sent again, so the same record can sit in the file twice.
func TestDecodeOTLPCollapsesARedeliveredEvent(t *testing.T) {
	input := export(proposed("a1", "ra")) + export(proposed("a1", "ra"), decided("a2", "ra", "a1"))
	events := decodeOTLP(t, input, 3)
	if got := eventIDs(events); got != "a1,a2" {
		t.Fatalf("events = %s, want a1,a2", got)
	}
}

// Two different records under one id cannot both be the trail, and choosing
// either would grade the run on a guess. Bytes decide, not meaning.
func TestDecodeOTLPRefusesOneIDWithTwoContents(t *testing.T) {
	other := logRecord(body("a1", "ra", "EVENT_KIND_POLICY_DECIDED", ""), attrs("a1", "ra", "EVENT_KIND_POLICY_DECIDED"))
	spaced := strings.Replace(proposed("a1", "ra"), `\"kind\":`, `\"kind\": `, 1)
	for name, second := range map[string]string{"another kind": other, "another spelling": spaced} {
		t.Run(name, func(t *testing.T) {
			input := export(proposed("a1", "ra")) + export(second)
			_, err := evidence.DecodeOTLP(strings.NewReader(input), namespace, 4)
			if !errors.Is(err, evidence.ErrEventConflict) {
				t.Fatalf("err = %v, want ErrEventConflict", err)
			}
		})
	}
}

// The limit counts records as delivered, duplicates included, because that is
// what the reader holds in memory.
func TestDecodeOTLPHoldsTheLimit(t *testing.T) {
	two := export(proposed("a1", "ra"), decided("a2", "ra", "a1"))
	if got := eventIDs(decodeOTLP(t, two, 2)); got != "a1,a2" {
		t.Fatalf("events = %s, want a1,a2", got)
	}
	cases := map[string]string{"distinct": two, "duplicate": export(proposed("a1", "ra"), proposed("a1", "ra"))}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := evidence.DecodeOTLP(strings.NewReader(input), namespace, 1)
			if !errors.Is(err, evidence.ErrTooManyEvents) {
				t.Fatalf("err = %v, want ErrTooManyEvents", err)
			}
		})
	}
}

func TestDecodeOTLPRefusesANegativeLimitAndAnEmptyNamespace(t *testing.T) {
	input := export(proposed("a1", "ra"))
	if _, err := evidence.DecodeOTLP(strings.NewReader(input), namespace, -1); !errors.Is(err, evidence.ErrInvalidLimit) {
		t.Errorf("limit -1: err = %v, want ErrInvalidLimit", err)
	}
	if _, err := evidence.DecodeOTLP(strings.NewReader(input), "", 1); !errors.Is(err, evidence.ErrInvalidNamespace) {
		t.Errorf("empty namespace: err = %v, want ErrInvalidNamespace", err)
	}
}

// A body at the line limit is read; one byte more is refused, never cut.
func TestDecodeOTLPHoldsTheLineLimitOnTheBody(t *testing.T) {
	line := body("a1", "ra", "EVENT_KIND_ACTION_PROPOSED", "")
	stub := strings.TrimSuffix(line, "}") + `,"schemaVersion":"`
	padded := func(size int) string {
		return stub + strings.Repeat("v", size-len(stub)-2) + `"}`
	}
	list := attrs("a1", "ra", "EVENT_KIND_ACTION_PROPOSED")

	atLimit := padded(evidence.MaxLineBytes)
	if len(atLimit) != 262144 {
		t.Fatalf("fixture is %d bytes, want 262144", len(atLimit))
	}
	if got := eventIDs(decodeOTLP(t, export(logRecord(atLimit, list)), 1)); got != "a1" {
		t.Fatalf("events = %s, want a1", got)
	}
	over := padded(evidence.MaxLineBytes + 1)
	_, err := evidence.DecodeOTLP(strings.NewReader(export(logRecord(over, list))), namespace, 1)
	if !errors.Is(err, evidence.ErrLineTooLong) {
		t.Fatalf("err = %v, want ErrLineTooLong", err)
	}
}

func TestDecodeOTLPReadsNothingFromNothing(t *testing.T) {
	events := decodeOTLP(t, "", 1)
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0", len(events))
	}
}
