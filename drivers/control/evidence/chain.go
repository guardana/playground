package evidence

import (
	"errors"
	"fmt"
)

var (
	// ErrChainBroken reports a definite defect: a link that does not join, an
	// event belonging to another request, project or tenant, or a step the
	// documented order does not allow.
	ErrChainBroken = errors.New("evidence: chain broken")

	// ErrChainIndeterminate reports a kind this reader cannot place, so the
	// order could not be established either way. The contract says an
	// undeclared kind is indeterminate to a reader and never an error, which is
	// what lets a later minor version add one. Reporting such a trail as well
	// formed would claim a reading nobody performed; reporting it as broken
	// would blame the producer for being newer.
	ErrChainIndeterminate = errors.New("evidence: chain indeterminate")
)

// ValidateChain reports whether events are one coherent account of one request.
//
// It answers whether the trail is well formed, not whether it is true:
// prevEventId is an ordering link, so a gap shows and an altered record does
// not. The order it accepts is the enforcer's own state diagram of a trail
// (docs/concepts/evidence-and-the-spool.md at the pin), a bracketed group being
// optional and a starred one repeated:
//
//	ACTION_PROPOSED -> POLICY_DECIDED
//	  -> [ APPROVAL_REQUESTED (APPROVAL_EXPIRED APPROVAL_REQUESTED)*
//	       -> ( APPROVAL_DECIDED | APPROVAL_EXPIRED -> ACTION_BLOCKED ) ]
//	  -> ( ACTION_STARTED -> (ACTION_COMPLETED | ACTION_FAILED) )
//	   | ACTION_BLOCKED
//
// An expired approval is followed by a new request or a block, never by a run.
//
// FINDING_RAISED is accepted anywhere after ACTION_PROPOSED, because a detector
// finishing is not this request progressing. POLICY_RELOADED is accepted
// anywhere at all, including first, because an operator caused it.
//
// A trail may end in any state: an in-flight request is a prefix, not a defect.
// An empty slice is refused, because that is the shape a dropped read has.
func ValidateChain(events []Event) error {
	if len(events) == 0 {
		return fmt.Errorf("%w: no events", ErrChainBroken)
	}
	// Links first: they hold whatever the order turns out to be, so a definite
	// defect is reported as definite even in a trail carrying a kind this
	// reader cannot place.
	if err := checkLinks(events); err != nil {
		return err
	}
	return checkOrder(events)
}

func checkLinks(events []Event) error {
	scope, err := scopeOf(events[0])
	if err != nil {
		return err
	}
	seen := make(map[string]int, len(events))
	prev := ""
	for i, event := range events {
		if err := scope.holds(i, event); err != nil {
			return err
		}
		if event.EventID == "" {
			// An empty id and an empty prevEventId are the same bytes as the
			// head of a trail, so the link below could not be read.
			return fmt.Errorf("%w: event %d carries no eventId", ErrChainBroken, i)
		}
		if firstAt, repeated := seen[event.EventID]; repeated {
			// A repeating id generator makes every link join, an event to
			// itself included, so the link check alone would report nothing.
			return fmt.Errorf("%w: events %d and %d share eventId %q",
				ErrChainBroken, firstAt, i, event.EventID)
		}
		seen[event.EventID] = i
		if event.PrevEventID != prev {
			return fmt.Errorf("%w: event %d links to %q, want %q",
				ErrChainBroken, i, event.PrevEventID, prev)
		}
		prev = event.EventID
	}
	return nil
}

func checkOrder(events []Event) error {
	state := chainStart
	for i, event := range events {
		next, result := state.step(event.Kind)
		switch result {
		case stepAllowed:
			state = next
		case stepUnknownKind:
			return fmt.Errorf("%w: event %d has kind %q, which this reader cannot place",
				ErrChainIndeterminate, i, event.Kind)
		case stepRefused:
			return fmt.Errorf("%w: event %d is %s, which cannot follow %s",
				ErrChainBroken, i, event.Kind, state)
		}
	}
	return nil
}

// chainState is where a request has got to. The zero value is the state before
// its first event, so a fresh walk needs no setup.
type chainState int

const (
	chainStart chainState = iota
	chainProposed
	chainDecided
	chainApprovalRequested
	chainApprovalDecided
	chainApprovalExpired
	chainStarted
	chainClosed
)

// String names the state for the refusal message, which is read by someone
// holding a file and no code.
func (s chainState) String() string {
	switch s {
	case chainStart:
		return "the start of a trail"
	case chainProposed:
		return string(KindActionProposed)
	case chainDecided:
		return string(KindPolicyDecided)
	case chainApprovalRequested:
		return string(KindApprovalRequested)
	case chainApprovalDecided:
		return string(KindApprovalDecided)
	case chainApprovalExpired:
		return string(KindApprovalExpired)
	case chainStarted:
		return string(KindActionStarted)
	case chainClosed:
		return "a closed action"
	default:
		return "an unnamed state"
	}
}

type stepResult int

const (
	stepAllowed stepResult = iota
	stepRefused
	// The kind is not one of the eleven this reader knows; see
	// ErrChainIndeterminate.
	stepUnknownKind
)

// step reports the state after kind. The returned state means nothing unless
// the result is stepAllowed.
func (s chainState) step(kind Kind) (chainState, stepResult) {
	switch kind {
	case KindActionProposed:
		return chainProposed, allow(s == chainStart)
	case KindPolicyDecided:
		return chainDecided, allow(s == chainProposed)
	case KindApprovalRequested:
		return chainApprovalRequested, allow(s.mayRequest())
	case KindApprovalDecided:
		return chainApprovalDecided, allow(s == chainApprovalRequested)
	case KindApprovalExpired:
		return chainApprovalExpired, allow(s == chainApprovalRequested)
	case KindActionStarted:
		return chainStarted, allow(s.mayRun())
	case KindActionCompleted, KindActionFailed:
		return chainClosed, allow(s == chainStarted)
	case KindActionBlocked:
		return chainClosed, allow(s.mayBlock())
	case KindFindingRaised:
		return s, allow(s != chainStart)
	case KindPolicyReloaded:
		return s, stepAllowed
	case KindUnspecified:
		// Declared, and it says nothing about the event carrying it. That is a
		// defect in the record, not a kind from a later version.
		return s, stepRefused
	default:
		return s, stepUnknownKind
	}
}

// mayRequest reports whether an approval may be asked for: after the verdict
// called for one, or again after the last one expired.
func (s chainState) mayRequest() bool {
	return s == chainDecided || s == chainApprovalExpired
}

// mayRun reports whether a verdict is recorded, nothing has run yet, and any
// approval it called for was granted.
func (s chainState) mayRun() bool {
	return s == chainDecided || s == chainApprovalDecided
}

// mayBlock reports whether the gateway may still refuse the call: where it
// could run, and after its approval expired.
func (s chainState) mayBlock() bool {
	return s.mayRun() || s == chainApprovalExpired
}

func allow(ok bool) stepResult {
	if ok {
		return stepAllowed
	}
	return stepRefused
}
