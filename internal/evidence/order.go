package evidence

import (
	"fmt"
	"slices"
)

// Each wraps ErrChainBroken, so a caller matching that still matches these.
var (
	// ErrChainFork reports two events that open one trail, or two that follow
	// the same event.
	ErrChainFork = fmt.Errorf("%w: fork", ErrChainBroken)

	// ErrChainCycle reports links that lead back on themselves.
	ErrChainCycle = fmt.Errorf("%w: cycle", ErrChainBroken)

	// ErrChainDangling reports a link to an event the input does not hold,
	// which is what a lost delivery looks like.
	ErrChainDangling = fmt.Errorf("%w: dangling link", ErrChainBroken)
)

// OrderChain puts one request's events in the order their prevEventId links
// give, whatever order they were delivered in. The links must form a single
// chain from one head: a fork, a cycle or a link to an absent event is
// refused, since any ordering of such a set would be a choice this reader made
// rather than one the producer recorded. The input is not modified.
//
// It orders; it does not validate. ValidateChain still judges the result.
func OrderChain(events []Event) ([]Event, error) {
	index, err := indexChain(events)
	if err != nil {
		return nil, err
	}
	head := -1
	next := make(map[string]int, len(events))
	for i, event := range events {
		prev := event.PrevEventID
		if prev == "" {
			if head >= 0 {
				return nil, fmt.Errorf("%w: events %q and %q both open the trail",
					ErrChainFork, events[head].EventID, event.EventID)
			}
			head = i
			continue
		}
		if _, held := index[prev]; !held {
			return nil, fmt.Errorf("%w: %q follows %q, which is absent", ErrChainDangling, event.EventID, prev)
		}
		if other, taken := next[prev]; taken {
			return nil, fmt.Errorf("%w: %q and %q both follow %q",
				ErrChainFork, events[other].EventID, event.EventID, prev)
		}
		next[prev] = i
	}
	if head < 0 {
		return nil, fmt.Errorf("%w: no event opens the trail", ErrChainCycle)
	}
	// Every event has one predecessor and none precedes the head, so this walk
	// cannot revisit an event; what it does not reach lies on a cycle.
	ordered := make([]Event, 0, len(events))
	for i, ok := head, true; ok; i, ok = next[events[i].EventID] {
		ordered = append(ordered, events[i])
	}
	if len(ordered) < len(events) {
		return nil, fmt.Errorf("%w: %d of %d events are not reached from %q",
			ErrChainCycle, len(events)-len(ordered), len(events), events[head].EventID)
	}
	return ordered, nil
}

// indexChain maps each eventId to its position, refusing what makes the links
// unreadable: no events, an empty or repeated id, a second request, project
// or tenant.
func indexChain(events []Event) (map[string]int, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("%w: no events", ErrChainBroken)
	}
	scope, err := scopeOf(events[0])
	if err != nil {
		return nil, err
	}
	index := make(map[string]int, len(events))
	for i, event := range events {
		if err := scope.holds(i, event); err != nil {
			return nil, err
		}
		if event.EventID == "" {
			return nil, fmt.Errorf("%w: event %d carries no eventId", ErrChainBroken, i)
		}
		if first, repeated := index[event.EventID]; repeated {
			return nil, fmt.Errorf("%w: events %d and %d share eventId %q",
				ErrChainBroken, first, i, event.EventID)
		}
		index[event.EventID] = i
	}
	return index, nil
}

// OrderByRequest groups events by request and orders each group with
// OrderChain. Events naming no request belong to no trail, so they are kept
// in the order delivered, under the empty key, where ValidateChain refuses
// them as a trail.
func OrderByRequest(events []Event) (map[string][]Event, error) {
	trails := ByRequest(events)
	requests := make([]string, 0, len(trails))
	for request := range trails {
		requests = append(requests, request)
	}
	slices.Sort(requests)
	for _, request := range requests {
		if request == "" {
			continue
		}
		ordered, err := OrderChain(trails[request])
		if err != nil {
			return nil, fmt.Errorf("request %q: %w", request, err)
		}
		trails[request] = ordered
	}
	return trails, nil
}
