package evidence_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/evidence"
)

// A line is one event. Anything else on it, or a JSON value that decodes into
// an empty event without complaint, is refused rather than read as one.
func TestDecodeRefusesALineThatIsNotOneObject(t *testing.T) {
	first := strings.SplitAfter(trail, "\n")[0]
	cases := map[string]string{
		"null":          "null\n",
		"trailing text": strings.TrimSuffix(first, "\n") + ` {"eventId":"e9"}` + "\n",
		"trailing tail": strings.TrimSuffix(first, "\n") + "}\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := evidence.DecodeJSONL(strings.NewReader(body), 4)
			if !errors.Is(err, evidence.ErrMalformedLine) {
				t.Fatalf("err = %v, want ErrMalformedLine", err)
			}
		})
	}
}

func TestDecodeAcceptsSpaceAroundTheObject(t *testing.T) {
	first := strings.SplitAfter(trail, "\n")[0]
	events := decode(t, "  "+strings.TrimSuffix(first, "\n")+" \r\n")
	if got := events[0].EventID; got != "e1" {
		t.Errorf("eventId = %q, want e1", got)
	}
}
