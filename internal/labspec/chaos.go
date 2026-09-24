package labspec

import (
	"fmt"
	"slices"
	"time"
)

// ChaosProfile is the compose profile holding the proxy a toxic runs through;
// a scenario names it exactly when it names a toxic.
const ChaosProfile = "chaos"

// The values a fault's keys take.
const (
	ToxicLatency  = "latency"
	ToxicHang     = "hang"
	CollectorDown = "down"
)

// Fault is one thing the runner breaks after the lab boots and before the
// trajectory runs, and mends after the replay and before the trail is drained.
// Exactly one key is set. A fault states no outcome: what the enforcer did
// under it is graded from the trail and the journals like any other run.
type Fault struct {
	// Toxic slows or holds one victim's answers on their way to the enforcer.
	Toxic *Toxic `json:"toxic,omitempty"`
	// Collector "down" stops the collector the enforcer exports its trail to,
	// and starts it again before the drain.
	Collector string `json:"collector,omitempty"`
	// Relist has a second client list the named victim's tools once, so a
	// victim that changes its tools on a later listing does it under the
	// enforcer's open session.
	Relist string `json:"relist,omitempty"`
}

// Toxic names the victim whose path goes through the proxy and what the proxy
// does to that victim's answers: `latency` delays each one by Latency, `hang`
// lets none through until the fault is lifted.
type Toxic struct {
	Victim  string   `json:"victim"`
	Type    string   `json:"type"`
	Latency Duration `json:"latency,omitempty"`
}

// Proxied names the victims whose path goes through the proxy.
func (s Scenario) Proxied() []string {
	var victims []string
	for _, fault := range s.Chaos {
		if fault.Toxic != nil {
			victims = append(victims, fault.Toxic.Victim)
		}
	}
	return victims
}

func (s Scenario) validateChaos() error {
	if slices.Contains(s.Profile, ChaosProfile) != (len(s.Proxied()) > 0) {
		return fmt.Errorf("%w: profile %s and a toxic go together", ErrInvalid, ChaosProfile)
	}
	if len(s.Chaos) > 0 && s.Gateway == nil {
		return fmt.Errorf("%w: chaos acts on the enforcer's paths, and the stub decides this run", ErrInvalid)
	}
	seen := map[string]bool{}
	for i, fault := range s.Chaos {
		name, err := fault.validate(i)
		if err != nil {
			return err
		}
		if seen[name] {
			return fmt.Errorf("%w: chaos[%d] repeats %s; one fault per path", ErrInvalid, i, name)
		}
		seen[name] = true
	}
	return nil
}

// validate returns the path the fault acts on, which two faults may not share.
func (f Fault) validate(i int) (string, error) {
	set := 0
	for _, named := range []bool{f.Toxic != nil, f.Collector != "", f.Relist != ""} {
		if named {
			set++
		}
	}
	switch {
	case set != 1:
		return "", fmt.Errorf("%w: chaos[%d] names %d faults, want one of toxic, collector or relist", ErrInvalid, i, set)
	case f.Toxic != nil:
		return "toxic on " + f.Toxic.Victim, f.Toxic.validate(i)
	case f.Collector != "":
		return "collector", oneOf(fmt.Sprintf("chaos[%d].collector", i), f.Collector, CollectorDown)
	case !upstreamName.MatchString(f.Relist):
		return "", fmt.Errorf("%w: chaos[%d].relist is %q, want victim-<name>", ErrInvalid, i, f.Relist)
	}
	return "relist of " + f.Relist, nil
}

func (t Toxic) validate(i int) error {
	latency := time.Duration(t.Latency)
	switch {
	case !upstreamName.MatchString(t.Victim):
		return fmt.Errorf("%w: chaos[%d].toxic.victim is %q, want victim-<name>", ErrInvalid, i, t.Victim)
	case t.Type == ToxicLatency && (latency < time.Millisecond || latency > MaxWait || latency%time.Millisecond != 0):
		// The proxy takes whole milliseconds; a fraction would be cut off.
		return fmt.Errorf("%w: chaos[%d].toxic.latency is %s, want whole milliseconds from 1ms to %s", ErrInvalid, i, latency, MaxWait)
	case t.Type == ToxicHang && latency != 0:
		return fmt.Errorf("%w: chaos[%d].toxic is a hang, which takes no latency", ErrInvalid, i)
	}
	return oneOf(fmt.Sprintf("chaos[%d].toxic.type", i), t.Type, ToxicLatency, ToxicHang)
}
