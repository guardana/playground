package evidence

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// MaxLineBytes is the longest line this reader will hold. It is the contract's
// envelope limit, because the largest thing an event carries is one envelope,
// and it is a JSON measure against a Protobuf one, so a line near the limit is
// refused rather than silently truncated.
const MaxLineBytes = 262144

var (
	// ErrMalformedLine reports a line that is not one JSON object of one event.
	// The wrapped cause is the codec's own message; match on this sentinel.
	ErrMalformedLine = errors.New("evidence: malformed line")

	// ErrLineTooLong reports a line over MaxLineBytes.
	ErrLineTooLong = errors.New("evidence: line too long")

	// ErrTooManyEvents reports input holding more events than the caller said
	// it would hold. It is never read as "take the first n": a trail read short
	// is a trail whose end nobody saw.
	ErrTooManyEvents = errors.New("evidence: too many events")

	// ErrInvalidLimit reports a negative limit, which is not read as unlimited.
	ErrInvalidLimit = errors.New("evidence: invalid limit")
)

// Int64 is a 64-bit protobuf integer as JSON carries it. protojson writes one
// as a string and accepts either form, so this reads either. Without it a
// well formed trail would be refused for spelling a number the way the
// contract's own encoder spells it.
type Int64 int64

// UnmarshalJSON accepts a JSON number or a JSON string holding one.
func (n *Int64) UnmarshalJSON(raw []byte) error {
	text := string(bytes.Trim(raw, `"`))
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("evidence: %q is not a 64-bit integer", raw)
	}
	*n = Int64(value)
	return nil
}

// DecodeJSONL reads at most limit events, one per line.
//
// It parses bytes the lab did not write, so it trusts none of them. A field
// this reader has no name for is refused rather than dropped: a producer that
// recorded more than the lab reads is a producer the lab cannot grade a run
// against. A blank line is refused for the same reason, and so is a last line
// with no newline, which is what a write cut short by a crash looks like.
func DecodeJSONL(r io.Reader, limit int) ([]Event, error) {
	if limit < 0 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidLimit, limit)
	}
	reader := bufio.NewReaderSize(r, MaxLineBytes+1)
	events := make([]Event, 0, min(limit, 64))

	for number := 1; ; number++ {
		line, err := reader.ReadSlice('\n')
		atEOF := errors.Is(err, io.EOF)
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			return nil, fmt.Errorf("line %d: %w: over %d bytes", number, ErrLineTooLong, MaxLineBytes)
		case err != nil && !atEOF:
			return nil, fmt.Errorf("line %d: %w", number, err)
		case atEOF && len(line) == 0:
			return events, nil
		}
		if len(events) >= limit {
			return nil, fmt.Errorf("line %d: %w: limit %d", number, ErrTooManyEvents, limit)
		}
		event, err := decodeLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", number, err)
		}
		events = append(events, event)
		if atEOF {
			return events, nil
		}
	}
}

func decodeLine(line []byte) (Event, error) {
	trimmed := bytes.TrimSpace(line)
	switch {
	case len(trimmed) == 0:
		return Event{}, fmt.Errorf("%w: blank", ErrMalformedLine)
	case trimmed[0] != '{':
		// null decodes into an empty event without an error.
		return Event{}, fmt.Errorf("%w: not a JSON object", ErrMalformedLine)
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	var event Event
	if err := decoder.Decode(&event); err != nil {
		return Event{}, fmt.Errorf("%w: %w", ErrMalformedLine, err)
	}
	// Decode stops after the first value, so a second one on the line would
	// otherwise pass unread.
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Event{}, fmt.Errorf("%w: content after the event", ErrMalformedLine)
	}
	return event, nil
}

// ByRequest groups events into one trail per request, keeping the order the
// file carried them in. That order is what ValidateChain is checking, so
// sorting here would hide the defect it looks for.
func ByRequest(events []Event) map[string][]Event {
	trails := make(map[string][]Event)
	for _, event := range events {
		trails[event.RequestID] = append(trails[event.RequestID], event)
	}
	return trails
}
