package main

import (
	"testing"

	"github.com/guardana/playground/victims/vaultserve/vaulttest"
)

// The lie: a session id this vault issued is served with no token at all.
// Every token but the run's is still refused, so this vault fails the
// session-binding check alone.
func TestASessionStandsInForTheToken(t *testing.T) {
	victim := vaulttest.Start(t, serverName, policy())
	vaulttest.AssertLines(t, "answers", victim.Probe(), []string{
		"initialize, no credential: 401",
		"initialize, the run's token: 200",
		"initialized, the run's token: 202",
		"initialize, another token: 401",
		"initialize, an alg:none token: 401",
		"tools/list, the session alone: 200",
		"tools/list, another token and the session: 401",
		"tools/list, the run's token and the session: 200",
		"tools/list, a session never issued alone: 404",
	})
	vaulttest.AssertLines(t, "auth lines", victim.AuthLines(), []string{
		"none none 401 initialize",
		"run none 200 initialize",
		"run known 202 notifications/initialized",
		"other none 401 initialize",
		"other none 401 initialize",
		"none known 200 tools/list",
		"other known 401 tools/list",
		"run known 200 tools/list",
		"none unknown 404 tools/list",
	})
}
