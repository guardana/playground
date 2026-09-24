package evidence_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/guardana/playground/internal/evidence"
)

// chainOf builds one well-linked, single-scope trail of kinds, so the only
// thing a case below can be refused for is the order.
func chainOf(kinds ...evidence.Kind) []evidence.Event {
	events := make([]evidence.Event, len(kinds))
	prev := ""
	for i, kind := range kinds {
		id := fmt.Sprintf("e%d", i+1)
		events[i] = evidence.Event{
			EventID: id, Kind: kind, RequestID: "r1", ProjectID: "p", TenantID: "t", PrevEventID: prev,
		}
		prev = id
	}
	return events
}

const (
	kProposed  = evidence.KindActionProposed
	kDecided   = evidence.KindPolicyDecided
	kRequested = evidence.KindApprovalRequested
	kApproved  = evidence.KindApprovalDecided
	kExpired   = evidence.KindApprovalExpired
	kStarted   = evidence.KindActionStarted
	kCompleted = evidence.KindActionCompleted
	kFailed    = evidence.KindActionFailed
	kBlocked   = evidence.KindActionBlocked
	kFinding   = evidence.KindFindingRaised
	kReloaded  = evidence.KindPolicyReloaded
	kUnset     = evidence.KindUnspecified
	kUnknown   = evidence.Kind("EVENT_KIND_FROM_A_LATER_MINOR")
)

var everyKind = []evidence.Kind{
	kProposed, kDecided, kRequested, kApproved, kExpired, kStarted,
	kCompleted, kFailed, kBlocked, kFinding, kReloaded, kUnset, kUnknown,
}

// diagramEdge is one arrow of the enforcer's state diagram of a trail, copied
// from its evidence-and-the-spool page by hand rather than derived from the
// code under test.
type diagramEdge struct {
	from string
	kind evidence.Kind
	to   string
}

var diagram = []diagramEdge{
	{"start", kProposed, "proposed"},
	{"proposed", kDecided, "decided"},
	{"decided", kRequested, "requested"},
	{"decided", kStarted, "started"},
	{"decided", kBlocked, "closed"},
	{"requested", kApproved, "approved"},
	{"requested", kExpired, "expired"},
	{"approved", kStarted, "started"},
	{"approved", kBlocked, "closed"},
	{"expired", kRequested, "requested"},
	{"expired", kBlocked, "closed"},
	{"started", kCompleted, "closed"},
	{"started", kFailed, "closed"},
}

// reachedBy is one trail that ends in each state of the diagram.
var reachedBy = map[string][]evidence.Kind{
	"start":     {},
	"proposed":  {kProposed},
	"decided":   {kProposed, kDecided},
	"requested": {kProposed, kDecided, kRequested},
	"approved":  {kProposed, kDecided, kRequested, kApproved},
	"expired":   {kProposed, kDecided, kRequested, kExpired},
	"started":   {kProposed, kDecided, kStarted},
	"closed":    {kProposed, kDecided, kBlocked},
}

// wantAfter says what the page says of kind in state: an arrow is taken,
// POLICY_RELOADED is taken from every state, FINDING_RAISED from every state
// but the start, an undeclared kind is indeterminate and anything else is
// refused.
func wantAfter(state string, kind evidence.Kind) error {
	switch {
	case kind == kUnknown:
		return evidence.ErrChainIndeterminate
	case kind == kReloaded, kind == kFinding && state != "start":
		return nil
	}
	for _, edge := range diagram {
		if edge.from == state && edge.kind == kind {
			return nil
		}
	}
	return evidence.ErrChainBroken
}

func checkStep(t *testing.T, prefix []evidence.Kind, state string, kind evidence.Kind) {
	t.Helper()
	trail := append(append([]evidence.Kind{}, prefix...), kind)
	err := evidence.ValidateChain(chainOf(trail...))
	want := wantAfter(state, kind)
	if want == nil && err != nil {
		t.Errorf("%v then %s: %v, want accepted", prefix, kind, err)
	}
	if want != nil && !errors.Is(err, want) {
		t.Errorf("%v then %s: err = %v, want %v", prefix, kind, err, want)
	}
}

// Every kind from every state, against the diagram's edge list.
func TestValidateChainFollowsTheDiagramFromEveryState(t *testing.T) {
	for state, prefix := range reachedBy {
		for _, kind := range everyKind {
			checkStep(t, prefix, state, kind)
		}
	}
}

// An arrow lands where the diagram says: the trail it leaves behaves, for
// every next kind, like the state the arrow points at.
func TestValidateChainLandsEachArrowInTheStateItPointsAt(t *testing.T) {
	for _, edge := range diagram {
		prefix := append(append([]evidence.Kind{}, reachedBy[edge.from]...), edge.kind)
		for _, kind := range everyKind {
			checkStep(t, prefix, edge.to, kind)
		}
	}
}

// The events that leave a trail where it is do so mid-approval as well.
func TestValidateChainLetsReloadsAndFindingsSitAnywhereAfterTheStart(t *testing.T) {
	trail := []evidence.Kind{
		kReloaded, kProposed, kFinding, kDecided, kReloaded, kRequested, kFinding,
		kExpired, kReloaded, kRequested, kApproved, kFinding, kStarted, kReloaded, kCompleted, kFinding,
	}
	if err := evidence.ValidateChain(chainOf(trail...)); err != nil {
		t.Fatalf("ValidateChain: %v", err)
	}
}
