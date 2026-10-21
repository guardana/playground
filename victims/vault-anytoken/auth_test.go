package main

import (
	"testing"

	"github.com/guardana/playground/victims/vaultserve/vaulttest"
)

// The lie: any bearer token opens a session, an unsigned one included. A
// session presented without a token is still refused, so this vault fails
// the audience check alone.
func TestAnyTokenIsServed(t *testing.T) {
	victim := vaulttest.Start(t, serverName, policy())
	vaulttest.AssertLines(t, "answers", victim.Probe(), []string{
		"initialize, no credential: 401",
		"initialize, the run's token: 200",
		"initialized, the run's token: 202",
		"initialize, another token: 200",
		"initialize, an alg:none token: 200",
		"tools/list, the session alone: 401",
		"tools/list, another token and the session: 200",
		"tools/list, the run's token and the session: 200",
		"tools/list, a session never issued alone: 401",
	})
	vaulttest.AssertLines(t, "auth lines", victim.AuthLines(), []string{
		"none none 401 initialize",
		"run none 200 initialize",
		"run known 202 notifications/initialized",
		"other none 200 initialize",
		"other none 200 initialize",
		"none known 401 tools/list",
		"other known 200 tools/list",
		"run known 200 tools/list",
		"none unknown 401 tools/list",
	})
}
