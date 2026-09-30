package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// errListing reports a listing the approver cannot use. It is never read as
// "nothing waits": a held call left unanswered because a listing was misread
// looks exactly like one the script left to expire.
var errListing = errors.New("approver: listing not understood")

// errNoPlane reports a directory no plane holds.
var errNoPlane = errors.New("approver: no plane holds the approvals directory")

// The fixed lines of `approvals list` at the pinned enforcer. They are matched
// exactly, so a command that changed its output fails here instead of being
// half understood.
const (
	planeHolds     = "a plane holds this directory"
	planeAbsent    = "no plane holds this directory, so nothing will consume an answer"
	unboundWarning = "the readable fields below come from a projection and are not bound to the\n" +
		"action digest beside them: the binding is over the authorized argument\n" +
		"bytes, which no record holds"
	stateWaiting      = "APPROVAL_STATE_PENDING"
	stateApproved     = "APPROVAL_STATE_APPROVED"
	stateRejected     = "APPROVAL_STATE_REJECTED"
	resolutionWaiting = "pending"
	// A field line is two spaces, the label padded to 14, a space, the value.
	labelEnd = 2 + 14
)

// entry is one record as the listing shows it.
type entry struct {
	id, state, resolution                           string
	action, resource, effectClass, principal, agent string
	answeredBy                                      string
	// readable is false when the listing printed no projection for the
	// record, or one describing another approval.
	readable bool
}

// waiting is a record nobody has answered and the plane has not closed.
func (e entry) waiting() bool {
	return e.state == stateWaiting && e.resolution == resolutionWaiting
}

type listing struct {
	planeHolds bool
	entries    []entry
}

// parseListing reads the standard output of `approvals list dir`.
func parseListing(out []byte, dir string) (listing, error) {
	text := string(out)
	if !strings.HasSuffix(text, "\n") {
		return listing{}, fmt.Errorf("%w: it does not end in a line break", errListing)
	}
	head, body, _ := strings.Cut(strings.TrimSuffix(text, "\n"), "\n\n")
	count, plane, err := parseHead(head, dir)
	if err != nil {
		return listing{}, err
	}
	l := listing{planeHolds: plane}
	if body != "" {
		for _, block := range strings.Split(body, "\n\n") {
			e, err := parseEntry(block)
			if err != nil {
				return listing{}, err
			}
			l.entries = append(l.entries, e)
		}
	}
	if len(l.entries) != count {
		return listing{}, fmt.Errorf("%w: it says %d records and holds %d", errListing, count, len(l.entries))
	}
	return l, nil
}

func parseHead(head, dir string) (int, bool, error) {
	lines := strings.SplitN(head, "\n", 3)
	if len(lines) != 3 || lines[2] != unboundWarning {
		return 0, false, fmt.Errorf("%w: its head is not the one the pinned command prints", errListing)
	}
	number, rest, _ := strings.Cut(lines[0], " ")
	count, err := strconv.Atoi(number)
	noun := "records"
	if count == 1 {
		noun = "record"
	}
	if err != nil || count < 0 || rest != noun+" in "+dir {
		return 0, false, fmt.Errorf("%w: %q is not a count of records in %s", errListing, lines[0], dir)
	}
	switch lines[1] {
	case planeHolds:
		return count, true, nil
	case planeAbsent:
		return count, false, nil
	}
	return 0, false, fmt.Errorf("%w: %q says nothing known about the plane", errListing, lines[1])
}

func parseEntry(block string) (entry, error) {
	lines := strings.Split(block, "\n")
	id, ok := strings.CutPrefix(lines[0], "approval ")
	if !ok || strings.TrimSpace(id) == "" || strings.ContainsAny(id, " \t") {
		return entry{}, fmt.Errorf("%w: %q does not open a record", errListing, lines[0])
	}
	fields := map[string]string{}
	for _, line := range lines[1:] {
		label, value, err := parseField(line)
		if err != nil {
			return entry{}, fmt.Errorf("approval %s: %w", id, err)
		}
		if _, twice := fields[label]; twice {
			return entry{}, fmt.Errorf("%w: approval %s shows %q twice", errListing, id, label)
		}
		fields[label] = value
	}
	for _, required := range []string{"state", "resolution", "request", "expires"} {
		if fields[required] == "" {
			return entry{}, fmt.Errorf("%w: approval %s shows no %s", errListing, id, required)
		}
	}
	_, unreadable := fields["readable"]
	return entry{
		id: id, state: fields["state"], resolution: fields["resolution"],
		action: fields["action"], resource: fields["resource"], effectClass: fields["effect class"],
		principal: fields["principal"], agent: fields["agent"], answeredBy: fields["answered by"], readable: !unreadable,
	}, nil
}

// knownLabel is every label the pinned command prints for a record.
func knownLabel(label string) bool {
	switch label {
	case "state", "resolution", "request", "action digest", "bundle digest", "expires",
		"answered by", "answered at", "readable", "principal", "agent", "action",
		"upstream", "resource", "effect class", "rule ids", "requested":
		return true
	}
	return false
}

func parseField(line string) (string, string, error) {
	if len(line) <= labelEnd || !strings.HasPrefix(line, "  ") || line[labelEnd] != ' ' {
		return "", "", fmt.Errorf("%w: %q is not a field line", errListing, line)
	}
	label := strings.TrimRight(line[2:labelEnd], " ")
	if !knownLabel(label) {
		return "", "", fmt.Errorf("%w: %q is not a field the pinned command prints", errListing, label)
	}
	return label, line[labelEnd+1:], nil
}
