package main

import (
	"context"
	"sort"

	"github.com/guardana/playground/internal/journal"
)

// The journal tools for what the approver saw rather than answered. Each is
// served, so an exhaustive count of the approver's journal names it.
const (
	// toolNoPlane is an approval a listing showed while no plane held the
	// directory. It is never answered.
	toolNoPlane = "no-plane"
	// toolWait is the first sighting of an approval a rule answers after a
	// delay.
	toolWait = "wait"
	// toolUnknown is an answer whose command did not run to its own exit, or
	// a record that did not show what became of it.
	toolUnknown = "unknown"
)

// settle reads one listing after the loop has stopped and journals what each
// answer cut short became. It answers nothing.
func (a *approver) settle(ctx context.Context) error {
	if len(a.unsure()) == 0 {
		return nil
	}
	l, err := a.list(ctx)
	if err != nil {
		return err
	}
	return a.followUp(l)
}

// unsure is every approval whose answer was cut short, in id order.
func (a *approver) unsure() []string {
	var ids []string
	for id, s := range a.seen {
		if s.unsure != nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// followUp journals, for each answer cut short, what its record shows in l,
// in listing order and then the ones l no longer holds.
func (a *approver) followUp(l listing) error {
	for i := range l.entries {
		e := &l.entries[i]
		if s := a.seen[e.id]; s != nil && s.unsure != nil {
			if err := a.shown(e.id, s, e); err != nil {
				return err
			}
		}
	}
	for _, id := range a.unsure() {
		if err := a.shown(id, a.seen[id], nil); err != nil {
			return err
		}
	}
	return nil
}

// shown journals one record read back after its answer was cut short: served
// under the answer when it carries that answer from this rule's approver,
// refused when nobody answered it, and unknown otherwise, absent included.
func (a *approver) shown(id string, s *sighting, e *entry) error {
	r := s.unsure
	s.unsure = nil
	if e == nil {
		return a.record(toolUnknown, journal.Served, "approval="+id+" answer="+string(r.Answer)+" record=absent")
	}
	shows := " record=" + e.state + "/" + e.resolution
	by := ""
	if e.answeredBy != "" {
		by = " answered_by=" + e.answeredBy
	}
	switch {
	case e.state == r.Answer.state() && e.answeredBy == r.ApproverID:
		return a.record(string(r.Answer), journal.Served, "approval="+id+shows+by)
	case e.state == stateWaiting && e.answeredBy == "":
		return a.record(string(r.Answer), journal.Refused, "approval="+id+shows)
	}
	return a.record(toolUnknown, journal.Served, "approval="+id+" answer="+string(r.Answer)+shows+by)
}
