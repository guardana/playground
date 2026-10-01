package evidence_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/evidence"
)

// A valid object without its newline is what a write cut short looks like:
// the bytes after it were never written, so the trail's end was never seen.
func TestDecodeRefusesALastLineWithoutItsNewline(t *testing.T) {
	first := strings.SplitAfter(trail, "\n")[0]
	for name, body := range map[string]string{
		"one event":       strings.TrimSuffix(first, "\n"),
		"the whole trail": strings.TrimSuffix(trail, "\n"),
	} {
		t.Run(name, func(t *testing.T) {
			if strings.HasSuffix(body, "\n") {
				t.Fatal("the body still ends in a newline")
			}
			events, err := evidence.DecodeJSONL(strings.NewReader(body), 64)
			if !errors.Is(err, evidence.ErrMalformedLine) || !strings.Contains(err.Error(), "newline") {
				t.Fatalf("DecodeJSONL = %d events, %v; want ErrMalformedLine naming the missing newline", len(events), err)
			}
		})
	}
}

func TestDecodeReadsATrailWhoseLinesAllEnd(t *testing.T) {
	if !strings.HasSuffix(trail, "\n") {
		t.Fatal("the fixture trail does not end its last line")
	}
	if events := decode(t, trail); len(events) == 0 {
		t.Error("a whole trail decoded to nothing")
	}
}
