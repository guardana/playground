package gateway_test

import (
	"errors"
	"testing"

	"github.com/guardana/playground/runner/gateway"
)

// The listener binds one address on agent-net. Every other form would open it
// on the enforcer's other networks too, where a victim could hold a session as
// the configured principal.
func TestAssembleRefusesAListenerThatIsNotOneAddress(t *testing.T) {
	for _, listener := range []string{
		"", "0.0.0.0:8080", "[::]:8080", ":8080", "enforcer:8080", "10.231.4.62", "10.231.4.62:0",
		"[::ffff:0.0.0.0]:8080", "[::ffff:10.231.4.62]:8080", "127.0.0.1:8080", "[::1]:8080", "224.0.0.1:8080",
		"255.255.255.255:8080", "8.8.8.8:8080",
	} {
		in := inputs(partial)
		in.Listener = listener
		if out, err := gateway.Assemble(in); !errors.Is(err, gateway.ErrInvalid) {
			t.Errorf("a listener at %q was assembled: %v\n%s", listener, err, out)
		}
	}
}
