package evidence_test

import (
	"errors"
	"testing"

	"github.com/guardana/playground/internal/evidence"
)

// ev is one event of project p, tenant t, as the enforcer scopes every trail.
func ev(id, request, prev string, kind evidence.Kind) evidence.Event {
	return evidence.Event{EventID: id, RequestID: request, ProjectID: "p", TenantID: "t", PrevEventID: prev, Kind: kind}
}

func scoped(event evidence.Event, project, tenant string) evidence.Event {
	event.ProjectID, event.TenantID = project, tenant
	return event
}

// With more than one batch in flight, a later event of a trail can reach the
// collector before an earlier one; the links, not the file, give the order.
func TestOrderChainFollowsTheLinks(t *testing.T) {
	shuffled := []evidence.Event{
		ev("e3", "r1", "e2", evidence.KindActionStarted),
		ev("e1", "r1", "", evidence.KindActionProposed),
		ev("e4", "r1", "e3", evidence.KindActionCompleted),
		ev("e2", "r1", "e1", evidence.KindPolicyDecided),
	}
	ordered, err := evidence.OrderChain(shuffled)
	if err != nil {
		t.Fatalf("OrderChain: %v", err)
	}
	if got := eventIDs(ordered); got != "e1,e2,e3,e4" {
		t.Fatalf("order = %s, want e1,e2,e3,e4", got)
	}
	if got := eventIDs(shuffled); got != "e3,e1,e4,e2" {
		t.Errorf("input reordered in place: %s", got)
	}
	if err := evidence.ValidateChain(ordered); err != nil {
		t.Errorf("ValidateChain: %v", err)
	}
}

func TestOrderChainRefusesWhatIsNotOneChain(t *testing.T) {
	head := ev("e1", "r1", "", evidence.KindActionProposed)
	cases := map[string]struct {
		events []evidence.Event
		want   error
	}{
		"two follow one":    {[]evidence.Event{head, ev("e2", "r1", "e1", ""), ev("e3", "r1", "e1", "")}, evidence.ErrChainFork},
		"two heads":         {[]evidence.Event{head, ev("e2", "r1", "", "")}, evidence.ErrChainFork},
		"dangling link":     {[]evidence.Event{head, ev("e3", "r1", "e2", "")}, evidence.ErrChainDangling},
		"loop off a head":   {[]evidence.Event{head, ev("e2", "r1", "e3", ""), ev("e3", "r1", "e2", "")}, evidence.ErrChainCycle},
		"self link":         {[]evidence.Event{head, ev("e2", "r1", "e2", "")}, evidence.ErrChainCycle},
		"loop with no head": {[]evidence.Event{ev("e1", "r1", "e2", ""), ev("e2", "r1", "e1", "")}, evidence.ErrChainCycle},
		"no events":         {nil, evidence.ErrChainBroken},
		"repeated id":       {[]evidence.Event{head, head}, evidence.ErrChainBroken},
		"no event id":       {[]evidence.Event{head, ev("", "r1", "e1", "")}, evidence.ErrChainBroken},
		"no request id":     {[]evidence.Event{ev("e1", "", "", "")}, evidence.ErrChainBroken},
		"two requests":      {[]evidence.Event{head, ev("e2", "r2", "e1", "")}, evidence.ErrChainBroken},
		"two tenants":       {[]evidence.Event{head, scoped(ev("e2", "r1", "e1", evidence.KindPolicyDecided), "p", "u")}, evidence.ErrChainBroken},
		"two projects":      {[]evidence.Event{head, scoped(ev("e2", "r1", "e1", ""), "q", "t")}, evidence.ErrChainBroken},
		"no project id":     {[]evidence.Event{scoped(head, "", "t")}, evidence.ErrChainBroken},
		"no tenant id":      {[]evidence.Event{scoped(head, "p", "")}, evidence.ErrChainBroken},
		"later no tenant":   {[]evidence.Event{head, scoped(ev("e2", "r1", "e1", ""), "p", "")}, evidence.ErrChainBroken},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := evidence.OrderChain(tc.events)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if !errors.Is(err, evidence.ErrChainBroken) {
				t.Errorf("err = %v does not match ErrChainBroken", err)
			}
		})
	}
}

// Requests are ordered one by one. An event that names no request is no
// trail's, so it is kept as delivered and left for ValidateChain to refuse.
func TestOrderByRequestOrdersEachTrail(t *testing.T) {
	events := []evidence.Event{
		ev("b2", "r2", "b1", evidence.KindPolicyDecided),
		ev("a2", "r1", "a1", evidence.KindPolicyDecided),
		ev("n2", "", "", evidence.KindPolicyReloaded),
		ev("b1", "r2", "", evidence.KindActionProposed),
		ev("n1", "", "", evidence.KindFindingRaised),
		ev("a1", "r1", "", evidence.KindActionProposed),
	}
	trails, err := evidence.OrderByRequest(events)
	if err != nil {
		t.Fatalf("OrderByRequest: %v", err)
	}
	if len(trails) != 3 {
		t.Fatalf("trails = %d, want 3", len(trails))
	}
	for request, want := range map[string]string{"r1": "a1,a2", "r2": "b1,b2", "": "n2,n1"} {
		if got := eventIDs(trails[request]); got != want {
			t.Errorf("request %q = %s, want %s", request, got, want)
		}
	}
}

func TestOrderByRequestRefusesABrokenTrail(t *testing.T) {
	events := []evidence.Event{
		ev("a1", "r1", "", evidence.KindActionProposed),
		ev("b1", "r2", "", evidence.KindActionProposed),
		ev("b3", "r2", "b2", evidence.KindActionStarted),
	}
	if _, err := evidence.OrderByRequest(events); !errors.Is(err, evidence.ErrChainDangling) {
		t.Fatalf("err = %v, want ErrChainDangling", err)
	}
}
