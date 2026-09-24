package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	// ErrMalformedOTLP reports input that is not a stream of OTLP/JSON
	// ExportLogsServiceRequest objects, or a log record whose body is not one
	// string.
	ErrMalformedOTLP = errors.New("evidence: malformed OTLP export")

	// ErrAttributeMismatch reports a log record whose namespaced attributes do
	// not repeat its event's identifiers exactly.
	ErrAttributeMismatch = errors.New("evidence: attributes disagree with the event")

	// ErrEventConflict reports two deliveries of one eventId whose lines are
	// not byte-identical.
	ErrEventConflict = errors.New("evidence: one eventId, two events")

	// ErrInvalidNamespace reports an empty attribute namespace.
	ErrInvalidNamespace = errors.New("evidence: invalid attribute namespace")
)

// The envelope is the collector's, not the contract's, so members this reader
// does not use are left unread: a collector release that adds one says nothing
// about the trail. Strictness applies to the event in each body.
type exportRequest struct {
	ResourceLogs *[]resourceLogs `json:"resourceLogs"`
}

type resourceLogs struct {
	ScopeLogs []scopeLogs `json:"scopeLogs"`
}

type scopeLogs struct {
	LogRecords []logRecord `json:"logRecords"`
}

type logRecord struct {
	Body       json.RawMessage `json:"body"`
	Attributes []keyValue      `json:"attributes"`
}

type keyValue struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

// DecodeOTLP reads the enforcer's trail as OTLP/JSON logs: a stream of
// ExportLogsServiceRequest objects, one per line as the collector's file
// exporter writes them or one whole document as the enforcer posts it.
//
// Every log record must be one event: its body a single string holding one
// JSONL line, and its attributes under namespace repeating the event's
// identifiers. Export is at-least-once, so a redelivered event is collapsed
// when its line is byte-identical to the first delivery and refused when it is
// not. Events come back in first-delivery order; OrderByRequest orders them.
// limit bounds the log records read, duplicates included.
func DecodeOTLP(r io.Reader, namespace string, limit int) ([]Event, error) {
	switch {
	case limit < 0:
		return nil, fmt.Errorf("%w: %d", ErrInvalidLimit, limit)
	case namespace == "":
		return nil, fmt.Errorf("%w: empty", ErrInvalidNamespace)
	}
	d := &deliveries{namespace: namespace, limit: limit, lines: make(map[string]string)}
	decoder := json.NewDecoder(r)
	for number := 1; ; number++ {
		var request exportRequest
		err := decoder.Decode(&request)
		switch {
		case errors.Is(err, io.EOF):
			return d.events, nil
		case err != nil:
			return nil, fmt.Errorf("export %d: %w: %w", number, ErrMalformedOTLP, err)
		case request.ResourceLogs == nil:
			return nil, fmt.Errorf("export %d: %w: no resourceLogs", number, ErrMalformedOTLP)
		}
		if err := d.addRequest(*request.ResourceLogs); err != nil {
			return nil, fmt.Errorf("export %d: %w", number, err)
		}
	}
}

type deliveries struct {
	namespace string
	limit     int
	read      int
	events    []Event
	lines     map[string]string
}

func (d *deliveries) addRequest(resources []resourceLogs) error {
	for _, resource := range resources {
		for _, scope := range resource.ScopeLogs {
			for _, rec := range scope.LogRecords {
				d.read++
				if d.read > d.limit {
					return fmt.Errorf("record %d: %w: limit %d", d.read, ErrTooManyEvents, d.limit)
				}
				if err := d.add(rec); err != nil {
					return fmt.Errorf("record %d: %w", d.read, err)
				}
			}
		}
	}
	return nil
}

func (d *deliveries) add(rec logRecord) error {
	line, err := stringValue(rec.Body)
	if err != nil {
		return fmt.Errorf("body: %w", err)
	}
	event, err := decodeBody(line)
	if err != nil {
		return err
	}
	if err := checkAttributes(event, rec.Attributes, d.namespace); err != nil {
		return err
	}
	prior, seen := d.lines[event.EventID]
	switch {
	case !seen:
		d.lines[event.EventID] = line
		d.events = append(d.events, event)
	case prior != line:
		return fmt.Errorf("%w: %q", ErrEventConflict, event.EventID)
	}
	return nil
}

// decodeBody holds a body to the rules of a JSONL line, and adds one: an event
// with no eventId cannot be told apart from a redelivery.
func decodeBody(line string) (Event, error) {
	switch {
	case len(line) > MaxLineBytes:
		return Event{}, fmt.Errorf("%w: body over %d bytes", ErrLineTooLong, MaxLineBytes)
	case strings.ContainsAny(line, "\r\n"):
		return Event{}, fmt.Errorf("%w: body holds more than one line", ErrMalformedLine)
	}
	event, err := decodeLine([]byte(line))
	if err != nil {
		return Event{}, err
	}
	if event.EventID == "" {
		return Event{}, fmt.Errorf("%w: no eventId", ErrMalformedLine)
	}
	return event, nil
}

// stringValue reads an OTLP AnyValue that must hold a string and nothing else.
func stringValue(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("%w: absent", ErrMalformedOTLP)
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return "", fmt.Errorf("%w: %w", ErrMalformedOTLP, err)
	}
	text, ok := members["stringValue"]
	if len(members) != 1 || !ok {
		return "", fmt.Errorf("%w: not a single stringValue", ErrMalformedOTLP)
	}
	// A JSON null unmarshals into a string without an error.
	if trimmed := bytes.TrimSpace(text); len(trimmed) == 0 || trimmed[0] != '"' {
		return "", fmt.Errorf("%w: stringValue is not a string", ErrMalformedOTLP)
	}
	var value string
	if err := json.Unmarshal(text, &value); err != nil {
		return "", fmt.Errorf("%w: %w", ErrMalformedOTLP, err)
	}
	return value, nil
}
